package startup_bind_error_test

import (
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOccupiedListenAddressExitsInsteadOfWaitingForSignal(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "coldchain-server")
	build := exec.Command("go", "build", "-o", binary, "./cmd/coldchain-server")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建服务失败: %v: %s", err, output)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-addr="+listener.Addr().String(), "-data="+t.TempDir())
	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatal("监听地址被占用后服务仍等待退出信号")
	}
	if err == nil {
		t.Fatal("监听地址被占用时服务应返回启动错误")
	}
}
