package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"

	"github.com/creack/pty"
)

func main() {
	slow := len(os.Args) > 1 && os.Args[1] == "slow"
	load := len(os.Args) > 2 && os.Args[2] == "load"
	var loads []*exec.Cmd
	if load {
		for i := 0; i < runtime.NumCPU()*2; i++ {
			c := exec.Command("pwsh", "-NoProfile", "-Command", "$e=(Get-Date).AddSeconds(90); while((Get-Date) -lt $e){ [math]::Sqrt(12345) | Out-Null }")
			c.Start()
			loads = append(loads, c)
		}
		time.Sleep(2 * time.Second)
	}
	delay := ""
	if slow {
		delay = "Start-Sleep -Milliseconds 700; "
	}
	script := "function prompt { " + delay + "Write-Host -NoNewline (\"`e]16162;P`a\"); 'PS> ' }; Set-PSReadLineKeyHandler -Key Enter -ScriptBlock { [Console]::Out.Write(\"`e]16162;B`a\"); [Microsoft.PowerShell.PSConsoleReadLine]::AcceptLine() }"
	cmd := exec.Command("pwsh", "-NoLogo", "-NoProfile", "-NoExit", "-Command", script)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	p, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 100})
	if err != nil {
		panic(err)
	}
	type ev struct {
		t    time.Time
		kind byte
	}
	events := make(chan ev, 1000)
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				b := buf[:n]
				for {
					i := bytes.Index(b, []byte("\x1b]16162;"))
					if i < 0 || i+10 > len(b) {
						break
					}
					events <- ev{time.Now(), b[i+8]}
					b = b[i+10:]
				}
			}
			if err != nil {
				return
			}
		}
	}()
	time.Sleep(3 * time.Second)
	for len(events) > 0 {
		<-events
	}
	var all []ev
	for i := 0; i < 15; i++ {
		p.Write([]byte("Start-Sleep -Milliseconds 300"))
		time.Sleep(100 * time.Millisecond)
		p.Write([]byte("Get-Date | Out-Null"))
		time.Sleep(3500 * time.Millisecond)
		for len(events) > 0 {
			all = append(all, <-events)
		}
	}
	var gaps []int
	for i := 0; i+1 < len(all); i++ {
		if all[i].kind == 'P' && all[i+1].kind == 'B' {
			gaps = append(gaps, int(all[i+1].t.Sub(all[i].t).Milliseconds()))
		}
	}
	sort.Ints(gaps)
	var seq []string
	var prev time.Time
	for _, e := range all {
		d := 0
		if !prev.IsZero() {
			d = int(e.t.Sub(prev).Milliseconds())
		}
		prev = e.t
		seq = append(seq, fmt.Sprintf("%c+%d", e.kind, d))
	}
	if len(seq) > 14 {
		seq = seq[:14]
	}
	fmt.Println("first events:", seq)
	fmt.Println("slow prompt:", slow, "load:", load, "P->B gaps (ms), sorted:", gaps)
	p.Write([]byte("exit\r"))
	time.Sleep(300 * time.Millisecond)
	cmd.Process.Kill()
	for _, c := range loads {
		c.Process.Kill()
	}
}
