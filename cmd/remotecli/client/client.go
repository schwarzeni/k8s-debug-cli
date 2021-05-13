package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	conn, err := net.Dial("tcp", "0.0.0.0:30000")
	if err != nil {
		fmt.Println("dial failed, err", err)
		return
	}
	defer conn.Close()

	if !terminal.IsTerminal(0) || !terminal.IsTerminal(1) {
		fmt.Errorf("stdin/stdout should be terminal")
	}
	// 这个应该是处理Ctrl + C 这种特殊键位，以及处理 stdin 输入
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer terminal.Restore(0, oldState)

	go func() {
		if _, err := io.Copy(conn, os.Stdin); err != nil {
			log.Println("io.Copy(conn, os.Stdin)", err)
		}
	}()
	if _, err := io.Copy(os.Stdout, conn); err != nil {
		log.Println("io.Copy(conn, os.Stdin)", err)
	}
	log.Println("client exit")
}
