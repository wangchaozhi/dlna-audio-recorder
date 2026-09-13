package config

import (
 "flag"
 "fmt"
 "net"
 "os"
 "path/filepath"
 "time"
)

type Config struct { ListenAddr string; AdvertiseIP string; DeviceName string; OutputDir string; TempDir string; FFMpeg string; UserAgent string; SegmentGrace time.Duration; HTTPTimeout time.Duration; SSDPInterval time.Duration; KeepRaw bool }
func Parse() (Config, error) { var c Config; flag.StringVar(&c.ListenAddr,"listen",":1400","HTTP listen address"); flag.StringVar(&c.AdvertiseIP,"advertise-ip","","LAN IP advertised to DLNA controllers (auto-detected if empty)"); flag.StringVar(&c.DeviceName,"name","DLNA Audio Recorder","DLNA renderer friendly name"); flag.StringVar(&c.OutputDir,"output","recordings","recording output directory"); flag.StringVar(&c.TempDir,"temp","","temporary directory (defaults to <output>/.tmp)"); flag.StringVar(&c.FFMpeg,"ffmpeg","ffmpeg","ffmpeg executable path"); flag.StringVar(&c.UserAgent,"user-agent","DLNA-Audio-Recorder/1.0","HTTP user agent for media requests"); flag.DurationVar(&c.SegmentGrace,"segment-grace",1500*time.Millisecond,"overlap retained around metadata boundaries"); flag.DurationVar(&c.HTTPTimeout,"http-timeout",15*time.Second,"upstream response/header timeout"); flag.DurationVar(&c.SSDPInterval,"ssdp-interval",30*time.Second,"SSDP alive notification interval"); flag.BoolVar(&c.KeepRaw,"keep-raw",false,"keep raw overlapped capture files after finalization"); flag.Parse(); if c.AdvertiseIP=="" { ip,err:=detectLANIP(); if err!=nil{return c,err}; c.AdvertiseIP=ip }; if c.TempDir=="" { c.TempDir=filepath.Join(c.OutputDir,".tmp") }; if err:=os.MkdirAll(c.OutputDir,0o755);err!=nil{return c,err}; if err:=os.MkdirAll(c.TempDir,0o755);err!=nil{return c,err}; return c,nil }
func detectLANIP()(string,error){ conn,err:=net.Dial("udp","8.8.8.8:80"); if err==nil { defer conn.Close(); if a,ok:=conn.LocalAddr().(*net.UDPAddr);ok&&a.IP!=nil{return a.IP.String(),nil} }; ifaces,_:=net.Interfaces(); for _,iface:=range ifaces { addrs,_:=iface.Addrs(); for _,addr:=range addrs { var ip net.IP; switch v:=addr.(type){case *net.IPNet:ip=v.IP;case *net.IPAddr:ip=v.IP}; if ip!=nil&&ip.To4()!=nil&&!ip.IsLoopback(){return ip.String(),nil} } }; return "",fmt.Errorf("could not detect LAN IP; pass -advertise-ip") }
