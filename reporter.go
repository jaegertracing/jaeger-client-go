package jaeger

import (
	"sync"
	"sync/atomic"
	"time"

	z "github.com/uber/jaeger-client-go/thrift/gen/zipkincore"

	"github.com/opentracing/opentracing-go"
)

// Reporter is called by the tracer when a span is completed to report the span to the tracing collector.
type Reporter interface {
	// Report submits a new span to collectors, possibly asynchronously and/or with buffering.
	Report(span *span)

	// Close does a clean shutdown of the reporter, flushing any traces that may be buffered in memory.
	Close()
}

// ------------------------------

type noopReporter struct{}

// NewNoopReporter creates a reporter that ignores all reported spans.
func NewNoopReporter() Reporter {
	return &noopReporter{}
}

// Report implements Report() method of Reporter by doing nothing.
func (r *noopReporter) Report(span *span) {
	// noop
}

// Close implements Close() method of Reporter by doing nothing.
func (r *noopReporter) Close() {
	// noop
}

// ------------------------------

type loggingReporter struct {
	logger Logger
}

// NewLoggingReporter creates a reporter that logs all reported spans to provided logger.
func NewLoggingReporter(logger Logger) Reporter {
	return &loggingReporter{logger}
}

// Report implements Report() method of Reporter by logging the span to the logger.
func (r *loggingReporter) Report(span *span) {
	r.logger.Infof("Reporting span %+v", span)
}

// Close implements Close() method of Reporter by doing nothing.
func (r *loggingReporter) Close() {
	// noop
}

// ------------------------------

// InMemoryReporter is used for testing, and simply collects spans in memory.
type InMemoryReporter struct {
	spans []opentracing.Span
	lock  sync.Mutex
}

// NewInMemoryReporter creates a reporter that stores spans in memory.
// NOTE: the Tracer should be created with options.PoolSpans = false.
func NewInMemoryReporter() *InMemoryReporter {
	return &InMemoryReporter{
		spans: make([]opentracing.Span, 0, 10),
	}
}

// Report implements Report() method of Reporter by storing the span in the buffer.
func (r *InMemoryReporter) Report(span *span) {
	r.lock.Lock()
	r.spans = append(r.spans, span)
	r.lock.Unlock()
}

// Close implements Close() method of Reporter by doing nothing.
func (r *InMemoryReporter) Close() {
	// noop
}

// SpansSubmitted returns the number of spans accumulated in the buffer.
func (r *InMemoryReporter) SpansSubmitted() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return len(r.spans)
}

// GetSpans returns accumulated spans as a copy of the buffer.
func (r *InMemoryReporter) GetSpans() []opentracing.Span {
	r.lock.Lock()
	defer r.lock.Unlock()
	copied := make([]opentracing.Span, len(r.spans))
	copy(copied, r.spans)
	return copied
}

// ------------------------------

type compositeReporter struct {
	reporters []Reporter
}

// NewCompositeReporter creates a reporter that ignores all reported spans.
func NewCompositeReporter(reporters ...Reporter) Reporter {
	return &compositeReporter{reporters: reporters}
}

// Report implements Report() method of Reporter by delegating to each underlying reporter.
func (r *compositeReporter) Report(span *span) {
	for _, reporter := range r.reporters {
		reporter.Report(span)
	}
}

// Close implements Close() method of Reporter by closing each underlying reporter.
func (r *compositeReporter) Close() {
	for _, reporter := range r.reporters {
		reporter.Close()
	}
}

// ------------------------------

const (
	defaultQueueSize           = 100
	defaultBatchSize           = 10
	defaultBufferFlushInterval = 10 * time.Second
)

type remoteReporter struct {
	ReporterOptions
	sender       Sender
	queue        chan *z.Span
	queueLength  int64 // signed because metric's gauge is signed
	queueDrained sync.WaitGroup
	flushSignal  chan *sync.WaitGroup
}

// ReporterOptions control behavior of the reporter
type ReporterOptions struct {
	// QueueSize is the size of internal queue where reported spans are stored before they are processed in the background
	QueueSize int
	// BufferFlushInterval is how often the buffer is force-flushed, even if it's not full
	BufferFlushInterval time.Duration
	// Logger is used to log errors of span submissions
	Logger Logger
	// Metrics is used to record runtime stats
	Metrics *Metrics
}

// NewRemoteReporter creates a new reporter that sends spans out of process by means of Sender
func NewRemoteReporter(sender Sender, options *ReporterOptions) Reporter {
	if options == nil {
		options = &ReporterOptions{}
	}
	if options.QueueSize <= 0 {
		options.QueueSize = defaultQueueSize
	}
	if options.BufferFlushInterval <= 0 {
		options.BufferFlushInterval = defaultBufferFlushInterval
	}
	if options.Logger == nil {
		options.Logger = NullLogger
	}
	if options.Metrics == nil {
		options.Metrics = NewMetrics(NullStatsReporter, nil)
	}

	reporter := &remoteReporter{
		ReporterOptions: *options,
		sender:          sender,
		queue:           make(chan *z.Span, options.QueueSize),
		flushSignal:     make(chan *sync.WaitGroup),
	}
	go reporter.processQueue()
	return reporter
}

// Report implements Report() method of Reporter.
// It passes the span to a background go-routine for submission to Jaeger.
func (r *remoteReporter) Report(span *span) {
	thriftSpan := buildThriftSpan(span)
	select {
	case r.queue <- thriftSpan:
		atomic.AddInt64(&r.queueLength, 1)
	default:
		r.Metrics.ReporterDropped.Inc(1)
	}
}

// Close implements Close() method of Reporter by waiting for the queue to be drained.
func (r *remoteReporter) Close() {
	r.queueDrained.Add(1)
	close(r.queue)
	r.queueDrained.Wait()
}

// processQueue reads spans from the queue, converts them to Thrift, and stores them in an internal buffer.
// When the buffer length reaches batchSize, it is flushed by submitting the accumulated spans to Jaeger.
// Buffer also gets flushed automatically every batchFlushInterval seconds, just in case the tracer stopped
// reporting new spans.
func (r *remoteReporter) processQueue() {
	timer := time.NewTicker(r.BufferFlushInterval)
	for {
		select {
		case span, ok := <-r.queue:
			if ok {
				atomic.AddInt64(&r.queueLength, -1)
				if flushed, err := r.sender.Append(span); err != nil {
					r.Metrics.ReporterFailure.Inc(int64(flushed))
					r.Logger.Error(err.Error())
				} else if flushed > 0 {
					r.Metrics.ReporterSuccess.Inc(int64(flushed))
					// to reduce the number of gauge stats, we only emit queue length on flush
					r.Metrics.ReporterQueueLength.Update(atomic.LoadInt64(&r.queueLength))
				}
			} else {
				// queue closed
				timer.Stop()
				r.flush()
				r.queueDrained.Done()
				return
			}
		case <-timer.C:
			r.flush()
		case wg := <-r.flushSignal: // for testing
			r.flush()
			wg.Done()
		}
	}
}

// flush causes the Sender to flush its accumulated spans and clear the buffer
func (r *remoteReporter) flush() {
	if flushed, err := r.sender.Flush(); err != nil {
		r.Metrics.ReporterFailure.Inc(int64(flushed))
		r.Logger.Error(err.Error())
	} else if flushed > 0 {
		r.Metrics.ReporterSuccess.Inc(int64(flushed))
	}
}
