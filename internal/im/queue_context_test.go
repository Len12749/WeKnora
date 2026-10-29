package im

import (
	"context"
	"testing"
	"time"
)

// TestQueuedRequestSurvivesHandlerCancellation 是事故回归：
// 飞书长连接「先 ACK 再异步处理」，handler 一返回调用方的 defer cancel() 就会取消
// 请求 ctx。排队任务若直接继承它，出队时会被当成已取消静默丢弃，用户看到的是
// 「发了消息没回」。任务 ctx 必须由 imOutboundContext 派生。
func TestQueuedRequestSurvivesHandlerCancellation(t *testing.T) {
	ran := make(chan struct{})
	q := newQAQueue(1, 8, 8, 0, func(req *qaRequest) {
		if err := req.ctx.Err(); err != nil {
			t.Errorf("queued request must not be cancelled: %v", err)
		}
		close(ran)
	}, nil)

	stopCh := make(chan struct{})
	q.Start(stopCh)
	defer func() {
		q.Stop()
		close(stopCh)
	}()

	// 模拟 HandleMessage：请求 ctx 是可取消的，任务 ctx 由它脱链派生。
	reqParent, cancelHandler := context.WithCancel(context.Background())
	reqCtx, reqCancel := context.WithCancel(imOutboundContext(reqParent))
	req := &qaRequest{
		ctx:     reqCtx,
		cancel:  reqCancel,
		msg:     &IncomingMessage{UserID: "ou_test"},
		userKey: "ch:ou_test:",
	}

	if _, err := q.Enqueue(req); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// handler 返回，调用方取消请求上下文。
	cancelHandler()

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("queued request was dropped after the handler returned")
	}
}

// TestQueuedRequestStopsWhenCancelled 反向约束：/stop 这类显式取消仍要生效。
func TestQueuedRequestStopsWhenCancelled(t *testing.T) {
	ran := make(chan struct{})
	q := newQAQueue(1, 8, 8, 0, func(req *qaRequest) { close(ran) }, nil)

	stopCh := make(chan struct{})
	q.Start(stopCh)
	defer func() {
		q.Stop()
		close(stopCh)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	req := &qaRequest{
		ctx:     ctx,
		cancel:  cancel,
		msg:     &IncomingMessage{UserID: "ou_test"},
		userKey: "ch:ou_test:",
	}
	if _, err := q.Enqueue(req); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	cancel()

	select {
	case <-ran:
		t.Fatal("explicitly cancelled request must not run")
	case <-time.After(300 * time.Millisecond):
	}
}
