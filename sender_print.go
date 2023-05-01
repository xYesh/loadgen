package main

import (
	"context"
	"time"
)

// make sure it implements Sender
var _ Sender = (*SenderPrint)(nil)

func ft(ts time.Time) string {
	return ts.Format("15:04:05.000")
}

type traceInfo struct {
	TraceId  string
	SpanId   string
	ParentId string
}

func (t *traceInfo) span(parent string) *traceInfo {
	return &traceInfo{
		TraceId:  t.TraceId,
		SpanId:   randID(4),
		ParentId: parent,
	}
}

type PrintSendable struct {
	TInfo     *traceInfo
	StartTime time.Time
	Fields    map[string]interface{}
	log       Logger
}

func (s *PrintSendable) Send() {
	endTime := time.Now()
	s.log.Printf("T:%6.6s S:%4.4s P%4.4s start:%v end:%v %v\n", s.TInfo.TraceId, s.TInfo.SpanId, s.TInfo.ParentId, ft(s.StartTime), ft(endTime), s.Fields)
}

type SenderPrint struct {
	spancount  int
	tracecount int
	log        Logger
}

func NewSenderPrint(log Logger, opts Options) Sender {
	return &SenderPrint{
		log: log,
	}
}

func (t *SenderPrint) Close() {
	t.log.Info("sender sent %d traces with %d spans\n", t.tracecount, t.spancount)
}

type PrintKey string

func (t *SenderPrint) CreateTrace(ctx context.Context, name string, fielder *Fielder, count int64) (context.Context, Sendable) {
	t.tracecount++
	t.spancount++
	tinfo := &traceInfo{
		TraceId:  randID(6),
		SpanId:   randID(4),
		ParentId: "",
	}
	ctx = context.WithValue(ctx, PrintKey("trace"), tinfo)
	return ctx, &PrintSendable{
		TInfo:  tinfo,
		Fields: fielder.GetFields(count),
		log:    t.log,
	}
}

func (t *SenderPrint) CreateSpan(ctx context.Context, name string, fielder *Fielder) (context.Context, Sendable) {
	t.spancount++
	tinfo := ctx.Value(PrintKey("trace")).(*traceInfo)
	ctx = context.WithValue(ctx, PrintKey("trace"), tinfo.span(tinfo.SpanId))
	return ctx, &PrintSendable{
		TInfo:     tinfo.span(tinfo.SpanId),
		StartTime: time.Now(),
		Fields:    fielder.GetFields(0),
		log:       t.log,
	}
}
