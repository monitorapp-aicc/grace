// +build !windows

package gracehttp

import (
        "context"
        "crypto/tls"
        "fmt"
        "log"
        "net"
        "net/http"
        "os"
        "os/exec"
        "os/signal"
        "strings"
        "sync"
        "syscall"
        "time"
)

// AppServer : Grace 환경 구성 구조체
type AppServer struct {
        s           *http.Server
        l           net.Listener
        n           net.Listener
        stat        *connStat
        restartFlag bool
        restartOnce sync.Once
        wg          *sync.WaitGroup
}

type connStat struct {
        sync.Mutex
        cnt int
}

const (
        // Used to indicate a graceful restart in the new process.
        envCountKey       = "LISTEN_FDS"
        envCountKeyPrefix = envCountKey + "="
)

func InitHandler(srv *http.Server) *AppServer {
        a := &AppServer{
                s:    srv,
                stat: &connStat{},
                wg:   &sync.WaitGroup{},
        }
        a.s.ConnState = a.connStateListener
        go a.signalHandler()
        return a
}

func (a *AppServer) connStateListener(c net.Conn, cs http.ConnState) {
        switch cs {
        case http.StateNew:
                a.stat.Lock()
                a.stat.cnt++
                a.stat.Unlock()
        case http.StateClosed:
                a.stat.Lock()
                a.stat.cnt--
                a.stat.Unlock()
        }
}

// ServerRun : Grace Server Run
func (a *AppServer) ServerRun() {
        var err error
        var addr *net.TCPAddr
        countStr := os.Getenv(envCountKey)
        if countStr == "" {
                a.restartFlag = false
        } else {
                a.restartFlag = true
        }
        if a.restartFlag {
                log.Print("main: Listening to existing file descriptor 3.")
                f := os.NewFile(3, "")
                a.l, err = net.FileListener(f)
                a.n = a.l
                if a.s.TLSConfig != nil {
                        a.l = tls.NewListener(a.l, a.s.TLSConfig)
                }
                f.Close()
        } else {
                addr, err = net.ResolveTCPAddr("tcp", a.s.Addr)
                if err != nil {
                        log.Fatalln(err)
                }
                a.l, err = net.ListenTCP("tcp", addr)
                if err != nil {
                        log.Fatalln(err)
                }
                a.n = a.l
                if a.s.TLSConfig != nil {
                        a.l = tls.NewListener(a.l, a.s.TLSConfig)
                }
        }
        a.s.Serve(a.l)
}

func (a *AppServer) signalHandler() {
        ch := make(chan os.Signal, 10)
        signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR2)
        for {
                sig := <-ch
                switch sig {
                case syscall.SIGINT, syscall.SIGTERM:
                        // this ensures a subsequent INT/TERM will trigger standard go behaviour of
                        // terminating.

                        signal.Stop(ch)
                        return
                case syscall.SIGUSR2:
                        a.graceRestart()
                }
        }
}

func (a *AppServer) graceRestart() {
        a.restartOnce.Do(func() {
                statsTicker := time.NewTicker(time.Second * time.Duration(1))
                go func() {
                        for {
                                select {
                                case <-statsTicker.C:
                                        if a.stat.cnt <= 0 {
                                                a.s.Shutdown(context.Background())
                                                return
                                        }
                                }
                        }
                }()
                file, err := a.n.(filer).File()
                if err != nil {
                        log.Println(err)
                        return
                }
                defer file.Close()
                argv0, err := exec.LookPath(os.Args[0])
                if err != nil {
                        log.Println(err)
                        return
                }
                var env []string
                for _, v := range os.Environ() {
                        if !strings.HasPrefix(v, envCountKeyPrefix) {
                                env = append(env, v)
                        }
                }
                env = append(env, fmt.Sprintf("%s%d", envCountKeyPrefix, 1))

                allFiles := append([]*os.File{os.Stdin, os.Stdout, os.Stderr}, file)
                process, err := os.StartProcess(argv0, os.Args, &os.ProcAttr{
                        Env:   env,
                        Files: allFiles,
                })
                if err != nil {
                        log.Println(err)
                        return
                }
                log.Println(process.Pid)
        })

}

type filer interface {
        File() (*os.File, error)
}
