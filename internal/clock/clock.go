// Package clock 提供可注入时钟，便于测试对时间相关规则进行确定性验证。
package clock

import (
	"sync"
	"time"
)

// Clock 抽象当前时间来源。
type Clock interface {
	Now() time.Time
}

// Real 返回真实系统时间（UTC）。
type Real struct{}

// Now 实现 Clock。
func (Real) Now() time.Time { return time.Now().UTC() }

// Fake 为测试用可调时钟，线程安全。
type Fake struct {
	mu sync.Mutex
	t  time.Time
}

// NewFake 创建固定在 t 的假时钟。
func NewFake(t time.Time) *Fake {
	return &Fake{t: t.UTC()}
}

// Now 实现 Clock。
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Advance 将假时钟推进 d。
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

// Set 将假时钟设置为 t。
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t.UTC()
}
