package operation

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestStageOfClassifiesWrappedAndTextErrors(t *testing.T) {
	if got := StageOf(Wrap(StageJump, errors.New("failed")), StageExecute); got != StageJump {
		t.Fatalf("wrapped stage = %q", got)
	}
	if got := StageOf(fmt.Errorf("SSH 失败: 主机密钥已变化"), StageExecute); got != StageTrust {
		t.Fatalf("trust stage = %q", got)
	}
	if got := StageOf(fmt.Errorf("网络连接失败: connection refused"), StageExecute); got != StageNetwork {
		t.Fatalf("network stage = %q", got)
	}
	if got := StageOf(fmt.Errorf("远程命令超时或取消: context deadline exceeded"), StageExecute); got != StageTimeout {
		t.Fatalf("timeout stage = %q", got)
	}
	if got := StageOf(fmt.Errorf("托管密钥需要先解锁 sshmd 密码库"), StageAuth); got != StageVault {
		t.Fatalf("vault stage = %q", got)
	}
	if got := StageOf(fmt.Errorf("主机 web01 未配置可用的认证凭据"), StageAuth); got != StageCredential {
		t.Fatalf("credential stage = %q", got)
	}
	if got := StageOf(Wrap(StageCredential, errors.New("主机 db01 未配置托管密钥")), StageAuth); got != StageCredential {
		t.Fatalf("wrapped credential stage = %q", got)
	}
	if got := Suggestion(StageCredential); got == "" || !strings.Contains(got, "未尝试连接") {
		t.Fatalf("credential suggestion = %q", got)
	}
}

func TestIsConnectionFailureIncludesCredential(t *testing.T) {
	for _, stage := range []FailureStage{StageResolve, StageNetwork, StageJump, StageTrust, StageCredential, StageAuth} {
		if !IsConnectionFailure(stage) {
			t.Fatalf("%s should be a connection failure stage", stage)
		}
	}
	for _, stage := range []FailureStage{StageExecute, StageTransfer, StageSession, StageTimeout, StageVault, StageConfig, StageConfirm} {
		if IsConnectionFailure(stage) {
			t.Fatalf("%s should not be a connection failure stage", stage)
		}
	}
}
