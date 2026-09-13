package sanitize

import "strings"
func FileName(s string) string { s=strings.TrimSpace(s); if s==""{return "unknown"}; bad:=`<>:"/\\|?*`+"\x00\r\n\t"; s=strings.Map(func(r rune) rune { if strings.ContainsRune(bad,r)||r<32{return '_'}; return r },s); s=strings.Trim(s,". "); if len([]rune(s))>100{s=string([]rune(s)[:100])}; if s==""{return "unknown"}; return s }
