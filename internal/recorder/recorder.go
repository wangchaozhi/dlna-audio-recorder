package recorder

import (
 "bufio"
 "context"
 "errors"
 "fmt"
 "io"
 "log/slog"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "strings"
 "sync"
 "time"
 "github.com/wangchaozhi/dlna-audio-recorder/internal/metadata"
 "github.com/wangchaozhi/dlna-audio-recorder/internal/sanitize"
)
type Config struct { OutputDir,TempDir,FFMpeg,UserAgent string; Grace,HTTPTimeout time.Duration; KeepRaw bool }
type Manager struct { cfg Config; log *slog.Logger; mu sync.Mutex; session *session }
type session struct { ctx context.Context; cancel context.CancelFunc; url string; events chan command; done chan struct{} }
type command struct { kind string; track metadata.Track }
type segment struct { track metadata.Track; path string; file *os.File; opened time.Time; boundary time.Time }
type chunk struct { at time.Time; data []byte }
type tailBuffer struct { grace time.Duration; chunks []chunk }
func New(cfg Config,log *slog.Logger)*Manager{return &Manager{cfg:cfg,log:log}}
func(m *Manager)SetTrack(t metadata.Track)error{if strings.TrimSpace(t.URI)==""{return errors.New("empty media URI")};m.mu.Lock();defer m.mu.Unlock();if m.session==nil||m.session.url!=t.URI{if m.session!=nil{m.session.cancel()};ctx,cancel:=context.WithCancel(context.Background());s:=&session{ctx:ctx,cancel:cancel,url:t.URI,events:make(chan command,16),done:make(chan struct{})};m.session=s;go m.run(s,t);return nil};select{case m.session.events<-command{kind:"rotate",track:t}:default:return errors.New("recorder command queue full")};return nil}
func(m *Manager)SetNext(t metadata.Track){m.mu.Lock();defer m.mu.Unlock();if m.session==nil{return};select{case m.session.events<-command{kind:"next",track:t}:default:m.log.Warn("dropping next-track hint")}}
func(m *Manager)Stop(){m.mu.Lock();s:=m.session;m.session=nil;m.mu.Unlock();if s!=nil{s.cancel();<-s.done}}
func(m *Manager)run(s *session,initial metadata.Track){defer close(s.done);req,err:=http.NewRequestWithContext(s.ctx,http.MethodGet,s.url,nil);if err!=nil{m.log.Error("create upstream request","err",err);return};req.Header.Set("User-Agent",m.cfg.UserAgent);req.Header.Set("Icy-MetaData","1");client:=&http.Client{Transport:&http.Transport{ResponseHeaderTimeout:m.cfg.HTTPTimeout}};resp,err:=client.Do(req);if err!=nil{if !errors.Is(err,context.Canceled){m.log.Error("open upstream","url",s.url,"err",err)};return};defer resp.Body.Close();if resp.StatusCode<200||resp.StatusCode>=300{m.log.Error("upstream rejected request","status",resp.Status);return};m.log.Info("recording stream","url",s.url,"content_type",resp.Header.Get("Content-Type"));seg,err:=m.openSegment(initial);if err!=nil{m.log.Error("open segment","err",err);return};var pending *metadata.Track;tail:=tailBuffer{grace:m.cfg.Grace};reader:=bufio.NewReaderSize(resp.Body,128*1024);buf:=make([]byte,64*1024);for{select{case cmd:=<-s.events:if cmd.kind=="next"{cp:=cmd.track;pending=&cp}else if cmd.kind=="rotate"&&cmd.track.Identity()!=seg.track.Identity(){seg=m.rotate(seg,cmd.track,&tail)};default:};n,rerr:=reader.Read(buf);if n>0{now:=time.Now();data:=append([]byte(nil),buf[:n]...);if _,err:=seg.file.Write(data);err!=nil{m.log.Error("write segment","err",err);break};tail.add(now,data)};select{case cmd:=<-s.events:switch cmd.kind{case"next":cp:=cmd.track;pending=&cp;case"rotate":nt:=cmd.track;if pending!=nil&&(nt.Title==""||nt.Identity()==seg.track.Identity()){nt=*pending};if nt.Identity()!=seg.track.Identity(){seg=m.rotate(seg,nt,&tail);pending=nil}};default:};if rerr!=nil{if !errors.Is(rerr,io.EOF)&&!errors.Is(rerr,context.Canceled){m.log.Warn("upstream ended","err",rerr)};break};select{case<-s.ctx.Done():goto done;default:}};done:m.closeAndFinalize(seg)}
func(m *Manager)rotate(old *segment,nt metadata.Track,tail *tailBuffer)*segment{old.boundary=time.Now();ns,err:=m.openSegment(nt);if err!=nil{m.log.Error("rotate: open new segment","err",err);return old};for _,c:=range tail.snapshot(){_,_=ns.file.Write(c.data)};if err:=old.file.Sync();err!=nil{m.log.Warn("sync old segment","err",err)};_=old.file.Close();go m.finalize(old);m.log.Info("track boundary","from",old.track.Title,"to",nt.Title);return ns}
func(m *Manager)openSegment(t metadata.Track)(*segment,error){stamp:=time.Now().Format("20060102-150405.000");base:=sanitize.FileName(t.Title);if t.Artist!=""{base=sanitize.FileName(t.Artist)+" - "+base};path:=filepath.Join(m.cfg.TempDir,stamp+" - "+base+".capture");f,err:=os.Create(path);if err!=nil{return nil,err};return &segment{track:t,path:path,file:f,opened:time.Now()},nil}
func(m *Manager)closeAndFinalize(s *segment){if s==nil{return};_=s.file.Sync();_=s.file.Close();m.finalize(s)}
func(m *Manager)finalize(s *segment){title:=sanitize.FileName(s.track.Title);if title=="unknown"{title="track"};base:=title;if s.track.Artist!=""{base=sanitize.FileName(s.track.Artist)+" - "+title};out:=uniquePath(filepath.Join(m.cfg.OutputDir,base+".m4a"));args:=[]string{"-hide_banner","-loglevel","error","-y","-i",s.path,"-map","0:a:0","-vn","-c:a","aac","-b:a","256k"};if s.track.Title!=""{args=append(args,"-metadata","title="+s.track.Title)};if s.track.Artist!=""{args=append(args,"-metadata","artist="+s.track.Artist)};if s.track.Album!=""{args=append(args,"-metadata","album="+s.track.Album)};args=append(args,out);cmd:=exec.Command(m.cfg.FFMpeg,args...);if b,err:=cmd.CombinedOutput();err!=nil{fallback:=strings.TrimSuffix(out,".m4a")+".capture";_=os.Rename(s.path,fallback);m.log.Error("ffmpeg finalize failed","err",err,"output",strings.TrimSpace(string(b)),"capture",fallback);return};if !m.cfg.KeepRaw{_=os.Remove(s.path)};m.log.Info("saved track","file",out,"title",s.track.Title)}
func(t *tailBuffer)add(at time.Time,data []byte){t.chunks=append(t.chunks,chunk{at:at,data:data});cutoff:=at.Add(-t.grace);i:=0;for i<len(t.chunks)&&t.chunks[i].at.Before(cutoff){i++};if i>0{t.chunks=append([]chunk(nil),t.chunks[i:]...)}}
func(t *tailBuffer)snapshot()[]chunk{return append([]chunk(nil),t.chunks...)}
func uniquePath(p string)string{if _,err:=os.Stat(p);os.IsNotExist(err){return p};ext:=filepath.Ext(p);stem:=strings.TrimSuffix(p,ext);for i:=2;;i++{q:=stem+" ("+strconv.Itoa(i)+")"+ext;if _,err:=os.Stat(q);os.IsNotExist(err){return q}}}
func(m *Manager)Status()string{m.mu.Lock();defer m.mu.Unlock();if m.session==nil{return "STOPPED"};return fmt.Sprintf("RECORDING %s",m.session.url)}
