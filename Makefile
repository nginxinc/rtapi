default: build_darwin

all: build_darwin build_linux build_windows

build_darwin:
	GOOS=darwin GOARCH=amd64 go build -mod vendor -o build/darwin/rtapi rtapi.go

build_linux:
	GOOS=linux GOARCH=amd64 go build -mod vendor -o build/linux/rtapi rtapi.go

build_windows:
	GOOS=windows GOARCH=amd64 go build -mod vendor -o build/windows/rtapi rtapi.go

clean:
	rm -rf build/
