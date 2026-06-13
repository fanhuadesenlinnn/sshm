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
}
