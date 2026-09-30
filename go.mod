module kinh-desktop

go 1.25.6

require (
	github.com/sirupsen/logrus v1.9.3
	github.com/wailsapp/wails/v3 v3.0.0-beta.23
)

require (
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jlaffaye/ftp v0.2.0 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/pkg/sftp v1.13.10 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

require (
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/coder/websocket v1.8.14 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mogumc/oshind v0.0.0
	golang.org/x/sys v0.46.0 // indirect
)

replace github.com/mogumc/oshind => ./third_party/oshind
