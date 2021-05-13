module github.com/schwarzeni/k8s-debug-cli

go 1.15

require (
	github.com/creack/pty v1.1.11
	github.com/google/martian v2.1.0+incompatible
	golang.org/x/crypto v0.0.0-20210506145944-38f3c27a63bf
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007 // indirect
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v0.19.2
	github.com/docker/docker v20.10.6+incompatible
)
