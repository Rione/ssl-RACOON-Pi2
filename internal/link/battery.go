package link

import "time"

// バッテリー電圧の判定。STM から届く電圧は 1V 刻みで上下に跳ねるため、
// 1 回の跳ねで警報が鳴り始めたり、逆に止まったりしないよう、
// 「その状態が一定時間続いたか」で判定する。
//
// 回数ではなく時間で数えるのは、呼び出し間隔が場所によって違うため。
// GPIO のアラーム判定は約 1 秒に 1 回、ステータス表示の判定は 125Hz で呼ばれる。
const (
	// この時間続けてしきい値以下なら警報を出す
	batteryLowHold = 3 * time.Second
	// 警報を解除する電圧の余裕。0.1V 単位なので 5 = 0.5V
	batteryRecoverMargin = 5
	// この時間続けて余裕を超えていれば警報を解除する
	batteryRecoverHold = 2 * time.Second
)

// now はテストから差し替えるための時計。
var now = time.Now

// batteryDebouncer は「しきい値以下が続いたか」と「回復が続いたか」を別々に数える。
type batteryDebouncer struct {
	lowSince  time.Time
	highSince time.Time
}

// below はしきい値以下が batteryLowHold の間続いたときだけ true を返す。
// volt が 0 のときは STM からまだ値が届いていないので判定しない。
func (d *batteryDebouncer) below(volt uint8, threshold int) bool {
	if volt == 0 || int(volt) > threshold {
		d.lowSince = time.Time{}
		return false
	}
	if d.lowSince.IsZero() {
		d.lowSince = now()
	}
	return now().Sub(d.lowSince) >= batteryLowHold
}

// recovered はしきい値 + 余裕を超えた状態が batteryRecoverHold の間続いたときだけ
// true を返す。跳ね上がった値 1 回で警報が止まらないようにするため。
func (d *batteryDebouncer) recovered(volt uint8, threshold int) bool {
	if volt == 0 || int(volt) <= threshold+batteryRecoverMargin {
		d.highSince = time.Time{}
		return false
	}
	if d.highSince.IsZero() {
		d.highSince = now()
	}
	return now().Sub(d.highSince) >= batteryRecoverHold
}

func (d *batteryDebouncer) reset() {
	d.lowSince = time.Time{}
	d.highSince = time.Time{}
}

// 用途ごとに別のカウンタを持つ。共有すると、呼び出し間隔の違う処理どうしで
// 数えかけの状態を壊し合うため。
var (
	alarmDebouncer          batteryDebouncer // ブザー/LED のアラーム用
	statusLowDebouncer      batteryDebouncer // ステータス表示（低電圧）用
	statusCriticalDebouncer batteryDebouncer // ステータス表示（危険域）用
)

// BatteryBelowThreshold はしきい値以下が続いたときだけ true を返す。
func BatteryBelowThreshold(volt uint8, threshold int) bool {
	return alarmDebouncer.below(volt, threshold)
}

// BatteryRecovered は警報を解除してよいほど電圧が戻ったかを返す。
func BatteryRecovered(volt uint8, threshold int) bool {
	return alarmDebouncer.recovered(volt, threshold)
}

// ResetBatteryDebounce は警報を抜けるときに呼ぶ。
func ResetBatteryDebounce() {
	alarmDebouncer.reset()
}
