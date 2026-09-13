package metadata

import (
 "encoding/xml"
 "html"
 "strconv"
 "strings"
 "time"
)
type Track struct { URI string; Title string; Artist string; Album string; AlbumArt string; Duration time.Duration; Raw string }
func (t Track) Identity() string { return strings.Join([]string{t.URI,t.Title,t.Artist,t.Album},"\x00") }
type didl struct { Items []struct { Title string `xml:"title"`; Artist string `xml:"artist"`; Album string `xml:"album"`; AlbumArt string `xml:"albumArtURI"`; Resources []struct { URI string `xml:",chardata"`; Duration string `xml:"duration,attr"` } `xml:"res"` } `xml:"item"` }
func Parse(uri,raw string) Track { t:=Track{URI:strings.TrimSpace(uri),Raw:raw}; data:=strings.TrimSpace(raw); if data==""{return t}; if strings.Contains(data,"&lt;"){data=html.UnescapeString(data)}; var d didl; if xml.Unmarshal([]byte(data),&d)==nil&&len(d.Items)>0 { it:=d.Items[0]; t.Title,t.Artist,t.Album,t.AlbumArt=strings.TrimSpace(it.Title),strings.TrimSpace(it.Artist),strings.TrimSpace(it.Album),strings.TrimSpace(it.AlbumArt); if t.URI==""&&len(it.Resources)>0{t.URI=strings.TrimSpace(it.Resources[0].URI)}; if len(it.Resources)>0{t.Duration=parseDuration(it.Resources[0].Duration)} }; return t }
func parseDuration(v string) time.Duration { p:=strings.Split(v,":"); if len(p)!=3{return 0}; h,_:=strconv.ParseFloat(p[0],64); m,_:=strconv.ParseFloat(p[1],64); s,_:=strconv.ParseFloat(p[2],64); return time.Duration((h*3600+m*60+s)*float64(time.Second)) }
