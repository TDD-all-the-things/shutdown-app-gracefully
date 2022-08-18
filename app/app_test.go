package app_test

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	. "github.com/TDD-all-the-things/shutdown-app-gracefully/app"
	"github.com/stretchr/testify/assert"
)

var (
	shutdownTimeout = 300 * time.Millisecond
	waitTimeout     = 100 * time.Millisecond
	callbackTimeout = 30 * time.Millisecond

	settleTime = 100 * time.Millisecond

	_ Server = &FakeServer{}
)

// quiesce waits until we can be reasonably confident that all pending signals
// have been delivered by the OS.
func quiesce() {
	// The kernel will deliver a signal as a thread returns
	// from a syscall. If the only active thread is sleeping,
	// and the system is busy, the kernel may not get around
	// to waking up a thread to catch the signal.
	// We try splitting up the sleep to give the kernel
	// many chances to deliver the signal.
	start := time.Now()
	for time.Since(start) < settleTime {
		time.Sleep(settleTime / 10)
	}
}

type FakeServer struct {
	name    string
	addr    string
	exit    chan struct{}
	timeout time.Duration
}

func NewLongRunningFakeServer(name string, addr string) Server {
	return NewFakeServer(name, addr, 10*time.Second)
}

func NewFakeServer(name string, addr string, timeout time.Duration) Server {
	return &FakeServer{name: name, addr: addr, exit: make(chan struct{}), timeout: timeout}
}

func (f *FakeServer) Name() string { return f.name }
func (f *FakeServer) Addr() string { return f.addr }
func (f *FakeServer) Handle(pattern string, handler http.Handler) {
	panic("handle method should not be called")
}

func (f *FakeServer) Start() error {
	select {
	case <-f.exit:
		return nil
	}
}

func (f *FakeServer) Stop(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(f.timeout):
		close(f.exit)
		return nil
	}
}

func TestApp_Exit_By_Interrupt(t *testing.T) {

	t.Parallel()

	// 通过环境变量区分运行于子进程中的测试用例
	if os.Getenv(t.Name()) == "1" {
		// 测试场景: app正常启动后发送两次信号,app退出返回指定退出码
		app, err := New([]Server{NewLongRunningFakeServer("biz", "any"), NewLongRunningFakeServer("adm", "any")})
		assert.NoError(t, err)
		app.StartAndServe()
		return
	}

	cmd, stdout, err := runThisTestInSubProcess(t)
	assert.NoError(t, err)

	go func() {
		// 等待子进程中的测试开始运行
		quiesce()
		// 发送信号
		cmd.Process.Signal(os.Interrupt)
		// 等待信号被接收
		quiesce()
		// 再次发送信号 走强制退出逻辑
		cmd.Process.Signal(os.Interrupt)
		// 等待信号被接收
		quiesce()
	}()

	err = cmd.Wait()
	if err, ok := err.(*exec.ExitError); ok {
		// 断言强制退出码
		assert.Equal(t, InterruptExitCode, err.ExitCode())
	}

	if stdout.Len() > 0 {
		t.Log("\n********\n" + stdout.String() + "********\n")
	}
}

func runThisTestInSubProcess(t *testing.T) (*exec.Cmd, *bytes.Buffer, error) {
	// 只有正常结束的测试才能生成测试覆盖率
	// os.Exit(xxx)都不会生成测试报告
	path, _ := os.Getwd()
	cmd := exec.Command(os.Args[0], "-test.v", "-test.count=1", "-test.run="+t.Name(), "-test.timeout=3s", "-test.outputdir="+path, "-test.coverprofile="+t.Name()+".out")
	// 将当前测试的名字设置为环境变量
	cmd.Env = append(os.Environ(), t.Name()+"=1")
	// 当前进程退出后,子进程一起退出
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// 绑定子进程的标准输出用于调试
	stdout := &bytes.Buffer{}
	cmd.Stdout = bufio.NewWriter(stdout)
	// 启动子进程运行测试
	err := cmd.Start()
	return cmd, stdout, err
}

func TestApp_Exit_By_Timeout(t *testing.T) {

	t.Parallel()

	if os.Getenv(t.Name()) == "1" {
		// 测试场景: app正常启动后,收到第一次信号后,开始优雅关闭整个流程的计时,超时后app退出返回指定退出码
		app, err := New([]Server{NewLongRunningFakeServer("biz", "any"), NewLongRunningFakeServer("adm", "any")},
			// 测试不宜过长,所以传入自定义时间
			WithShutdownTimeout(shutdownTimeout))
		assert.NoError(t, err)
		app.StartAndServe()
		return
	}

	cmd, stdout, err := runThisTestInSubProcess(t)
	assert.NoError(t, err)

	go func() {
		// 等待子进程中的测试开始运行
		quiesce()
		// 发送信号
		cmd.Process.Signal(os.Interrupt)
		// 等待信号被接收
		quiesce()
	}()

	err = cmd.Wait()
	if err, ok := err.(*exec.ExitError); ok {
		// 断言超时退出码
		assert.Equal(t, TimeoutExitCode, err.ExitCode())
	}

	if stdout.Len() > 0 {
		t.Log("\n********\n" + stdout.String() + "********\n")
	}
}

func TestApp_Exit_Gracefully(t *testing.T) {

	t.Parallel()

	if os.Getenv(t.Name()) == "1" {

		fakeCallbackA := func(ctx context.Context) {
			select {
			case <-ctx.Done():
				return
			default:
				return
			}
		}
		fakeCallbackB := func(ctx context.Context) {
			select {
			case <-ctx.Done():
				return
			default:
				return
			}
		}

		// 测试场景:
		// app正常启动后,收到第一次信号后,开始优雅关闭整个流程的计时,在计时内不在发送信号,且所有Server和callback均按时、正常退出,至此app优雅关闭完成
		// 通过配置,使FakeServer和FakeCallback均运行短时间后退出
		app, err := New([]Server{NewFakeServer("biz", "any", waitTimeout), NewFakeServer("adm", "any", waitTimeout)},
			WithShutdownCallbacks(fakeCallbackA, fakeCallbackB),
			WithShutdownTimeout(shutdownTimeout),
			WithWaitTimeout(waitTimeout),
			WithCallbackTimeout(callbackTimeout))

		assert.NoError(t, err)
		app.StartAndServe()
		return
	}

	cmd, stdout, err := runThisTestInSubProcess(t)
	assert.NoError(t, err)

	go func() {
		// 等待子进程中的测试开始运行
		quiesce()
		// 发送信号
		cmd.Process.Signal(os.Interrupt)
		// 等待信号被接收
		quiesce()
	}()

	// 等待子进程中的测试运行结束
	err = cmd.Wait()
	assert.NoError(t, err)

	if stdout.Len() > 0 {
		t.Log("\n********\n" + stdout.String() + "********\n")
	}
}
