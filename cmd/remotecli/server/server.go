package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
)

func handleConn(conn net.Conn) {
	ptmx, tty, _ := pty.Open()

	// Handle pty size. 否则类似于 htop 的命令，图像界面就无法正常显示
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Printf("error resizing pty: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	go func() {
		c := exec.Command("/bin/bash")
		c.SysProcAttr = &syscall.SysProcAttr{}
		c.SysProcAttr.Setsid = true
		c.SysProcAttr.Setctty = true
		c.Stdin = tty
		c.Stdout = tty
		c.Stderr = tty
		_ = c.Start()
		_ = c.Wait()
		_ = ptmx.Close()
		signal.Stop(ch)
		close(ch)
		conn.Close()
	}()

	go func() {
		_, _ = io.Copy(ptmx, conn)
	}()
	_, _ = io.Copy(conn, ptmx)
}

func main() {
	listen, err := net.Listen("tcp", "0.0.0.0:30000")
	if err != nil {
		fmt.Println("listen failed, err:", err)
		return
	}
	defer listen.Close()
	conn, err := listen.Accept()
	if err != nil {
		fmt.Println("accept failed, err:", err)
		return
	}
	handleConn(conn)
}
