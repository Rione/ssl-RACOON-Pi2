package link

import (
	"testing"
	"time"
)

// fakeClock は now を差し替えて、時間の経過をテストから制御する。
func fakeClock(t *testing.T) func(time.Duration) {
	t.Helper()
	current := time.Unix(0, 0)
	now = func() time.Time { return current }
	t.Cleanup(func() { now = time.Now })
	return func(d time.Duration) { current = current.Add(d) }
}

func TestBelowNeedsSustainedLowVoltage(t *testing.T) {
	advance := fakeClock(t)
	var d batteryDebouncer

	if d.below(200, 200) {
		t.Fatal("1 回下回っただけで警報にしてはいけない")
	}
	advance(batteryLowHold - time.Millisecond)
	if d.below(200, 200) {
		t.Fatal("規定時間に達する前に警報にしてはいけない")
	}
	advance(time.Millisecond)
	if !d.below(200, 200) {
		t.Fatal("規定時間続いたら警報にする")
	}
}

func TestBelowResetsOnSpike(t *testing.T) {
	advance := fakeClock(t)
	var d batteryDebouncer

	d.below(200, 200)
	advance(batteryLowHold - time.Millisecond)

	// 1 回だけ跳ね上がった値が来たら、数え直しになる。
	if d.below(230, 200) {
		t.Fatal("しきい値を超えた時点で false を返すべき")
	}
	advance(time.Millisecond)
	if d.below(200, 200) {
		t.Fatal("跳ねのあとは最初から数え直す")
	}
	advance(batteryLowHold)
	if !d.below(200, 200) {
		t.Fatal("数え直したあと、規定時間続けば警報にする")
	}
}

func TestZeroVoltIsIgnored(t *testing.T) {
	advance := fakeClock(t)
	var d batteryDebouncer

	// 起動直後など、STM からまだ値が届いていない状態。
	d.below(0, 200)
	advance(batteryLowHold * 2)
	if d.below(0, 200) {
		t.Fatal("0V は判定に使ってはいけない")
	}
	if d.recovered(0, 200) {
		t.Fatal("0V で回復と判定してはいけない")
	}
}

func TestRecoveredNeedsMarginAndTime(t *testing.T) {
	advance := fakeClock(t)
	var d batteryDebouncer

	// しきい値 20.0V。余裕 0.5V を超えない 20.5V では解除しない。
	d.recovered(205, 200)
	advance(batteryRecoverHold * 2)
	if d.recovered(205, 200) {
		t.Fatal("余裕を超えていないので解除してはいけない")
	}

	// 20.6V は余裕を超えるが、続かないと解除しない。
	if d.recovered(206, 200) {
		t.Fatal("1 回超えただけで解除してはいけない")
	}
	advance(batteryRecoverHold)
	if !d.recovered(206, 200) {
		t.Fatal("規定時間続いたら解除する")
	}
}

func TestRecoveredResetsOnDip(t *testing.T) {
	advance := fakeClock(t)
	var d batteryDebouncer

	d.recovered(210, 200)
	advance(batteryRecoverHold - time.Millisecond)

	// 一瞬下がったら数え直し。跳ね 1 回で警報が止まらないようにするため。
	if d.recovered(190, 200) {
		t.Fatal("下がった時点で false を返すべき")
	}
	advance(time.Millisecond)
	if d.recovered(210, 200) {
		t.Fatal("下がったあとは最初から数え直す")
	}
}

func TestResetClearsBothCounters(t *testing.T) {
	advance := fakeClock(t)
	var d batteryDebouncer

	d.below(200, 200)
	d.recovered(210, 200)
	d.reset()

	advance(batteryLowHold + batteryRecoverHold)
	if d.below(200, 200) {
		t.Fatal("reset 後は最初から数え直す（低電圧側）")
	}
	if d.recovered(210, 200) {
		t.Fatal("reset 後は最初から数え直す（回復側）")
	}
}
