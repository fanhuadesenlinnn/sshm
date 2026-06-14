package operation

import (
	"errors"
	"fmt"
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
	if got := StageOf(fmt.Errorf("托管密钥需要先解锁 sshm 密码库"), StageAuth); got != StageVault {
		t.Fatalf("vault stage = %q", got)
	}
}
