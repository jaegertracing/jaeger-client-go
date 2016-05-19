package jaeger

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/uber/jaeger-client-go/thrift/gen/sampling"
	"github.com/uber/jaeger-client-go/utils"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
)

// Sampler decides whether a new trace should be sampled or not.
type Sampler interface {
	IsSampled(id uint64) bool

	// Close does a clean shutdown of the sampler, stopping any background go-routines it may have started.
	Close()

	// Equal checks if the `other` sampler is functionally equivalent to this sampler
	Equal(other Sampler) bool
}

type constSampler struct {
	decision bool
}

// NewConstSampler creates a sampler that always makes the same decision equal to the 'sample' argument.
func NewConstSampler(sample bool) Sampler {
	return &constSampler{decision: sample}
}

func (s *constSampler) IsSampled(id uint64) bool {
	return s.decision
}

func (s *constSampler) Close() {
	// nothing to do
}

func (s *constSampler) Equal(other Sampler) bool {
	if o, ok := other.(*constSampler); ok {
		return s.decision == o.decision
	}
	return false
}

// -----------------------

type probabilisticSampler struct {
	samplingBoundary uint64
}

var maxRandomNumber = ^(uint64(1) << 63) // i.e. 7fffffffffffffff

// NewProbabilisticSampler creates a sampler that randomly samples a certain percentage of traces specified by the
// samplingRate, in the range between 0.0 and 1.0.
//
// It relies on the fact that new trace IDs are 63bit random numbers themselves, thus making the sampling decision
// without generating a new random number, but simply calculating if traceID < (samplingRate * 2^63).
func NewProbabilisticSampler(samplingRate float64) (Sampler, error) {
	if samplingRate < 0.0 || samplingRate > 1.0 {
		return nil, fmt.Errorf("Sampling Rate must be between 0.0 and 1.0, received %f", samplingRate)
	}
	return &probabilisticSampler{uint64(float64(maxRandomNumber) * samplingRate)}, nil
}

// IsSampled implements IsSampled() of Sampler.
func (s *probabilisticSampler) IsSampled(id uint64) bool {
	return s.samplingBoundary >= id
}

func (s *probabilisticSampler) Close() {
	// nothing to do
}

func (s *probabilisticSampler) Equal(other Sampler) bool {
	if o, ok := other.(*probabilisticSampler); ok {
		return s.samplingBoundary == o.samplingBoundary
	}
	return false
}

// -----------------------

type rateLimitingSampler struct {
	maxTracesPerSecond float64
	rateLimiter        utils.RateLimiter
}

// NewRateLimitingSampler creates a sampler that samples at most maxTracesPerSecond. The distribution of sampled
// traces follows burstiness of the service, i.e. a service with uniformly distributed requests will have those
// requests sampled uniformly as well, but if requests are bursty, especially sub-second, then a number of
// sequential requests can be sampled each second.
func NewRateLimitingSampler(maxTracesPerSecond float64) (Sampler, error) {
	return &rateLimitingSampler{
		maxTracesPerSecond: maxTracesPerSecond,
		rateLimiter:        utils.NewRateLimiter(maxTracesPerSecond),
	}, nil
}

// IsSampled implements IsSampled() of Sampler.
func (s *rateLimitingSampler) IsSampled(id uint64) bool {
	return s.rateLimiter.CheckCredit(1.0)
}

func (s *rateLimitingSampler) Close() {
	// nothing to do
}

func (s *rateLimitingSampler) Equal(other Sampler) bool {
	if o, ok := other.(*rateLimitingSampler); ok {
		return s.maxTracesPerSecond == o.maxTracesPerSecond
	}
	return false
}

// -----------------------

type remoteControlledSampler struct {
	serviceName string
	manager     sampling.SamplingManager
	logger      Logger
	timer       *time.Ticker
	lock        sync.RWMutex
	sampler     Sampler
	pollStopped sync.WaitGroup
	metrics     Metrics
}

type tcollectorSamplingManager struct {
	client sampling.TChanSamplingManager
}

func (s *tcollectorSamplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	ctx, cancel := tchannel.NewContextBuilder(time.Second).DisableTracing().Build()
	defer cancel()
	return s.client.GetSamplingStrategy(ctx, serviceName)
}

// NewTCollectorControlledSampler creates a sampler that periodically pulls
// the sampling strategy from Jaeger collectors.
func NewTCollectorControlledSampler(
	serviceName string,
	initial Sampler,
	ch *tchannel.Channel,
	metrics *Metrics,
	logger Logger,
) Sampler {
	thriftClient := thrift.NewClient(ch, defaultTChannelServiceName, nil)
	client := sampling.NewTChanSamplingManagerClient(thriftClient)
	manager := &tcollectorSamplingManager{client}
	return newRemoteControlledSampler(manager, serviceName, initial, metrics, logger)
}

type httpSamplingManager struct {
	serverURL string
}

func (s *httpSamplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	var out sampling.SamplingStrategyResponse
	v := url.Values{}
	v.Set("service", serviceName)
	if err := utils.GetJSON(s.serverURL+"/?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NewHTTPRemoteControlledSampler creates a sampler that periodically pulls
// the sampling strategy from an HTTP sampling server (e.g. jaeger-agent).
func NewHTTPRemoteControlledSampler(
	serviceName string,
	initial Sampler,
	hostPort string,
	metrics *Metrics,
	logger Logger,
) Sampler {
	if hostPort == "" {
		hostPort = defaultSamplingServerHostPort
	}
	manager := &httpSamplingManager{serverURL: "http://" + hostPort}
	return newRemoteControlledSampler(manager, serviceName, initial, metrics, logger)
}

// newRemoteControlledSampler creates a sampler that periodically pulls
// the sampling strategy from a remote server via `manager`.
func newRemoteControlledSampler(
	manager sampling.SamplingManager,
	serviceName string,
	initial Sampler,
	metrics *Metrics,
	logger Logger,
) Sampler {
	if logger == nil {
		logger = NullLogger
	}
	sampler := &remoteControlledSampler{
		serviceName: serviceName,
		manager:     manager,
		logger:      logger,
		metrics:     *metrics,
		timer:       time.NewTicker(1 * time.Minute),
	}
	if initial != nil {
		sampler.sampler = initial
	} else {
		sampler.sampler, _ = NewProbabilisticSampler(0.001)
	}
	go sampler.pollController()
	return sampler
}

// IsSampled implements IsSampled() of Sampler.
func (s *remoteControlledSampler) IsSampled(id uint64) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.sampler.IsSampled(id)
}

func (s *remoteControlledSampler) Close() {
	s.lock.RLock()
	s.timer.Stop()
	s.lock.RUnlock()

	s.pollStopped.Wait()
}

func (s *remoteControlledSampler) Equal(other Sampler) bool {
	if o, ok := other.(*remoteControlledSampler); ok {
		s.lock.RLock()
		o.lock.RLock()
		defer s.lock.RUnlock()
		defer o.lock.RUnlock()
		return s.sampler.Equal(o.sampler)
	}
	return false
}

func (s *remoteControlledSampler) pollController() {
	// in unit tests we re-assign the timer Ticker, so need to lock to avoid data races
	s.lock.Lock()
	timer := s.timer
	s.lock.Unlock()

	for range timer.C {
		s.updateSampler()
	}
	s.pollStopped.Add(1)
}

func (s *remoteControlledSampler) updateSampler() {
	if res, err := s.manager.GetSamplingStrategy(s.serviceName); err == nil {
		if sampler, err := s.extractSampler(res); err == nil {
			s.metrics.SamplerRetrieved.Inc(1)
			if !s.sampler.Equal(sampler) {
				s.lock.Lock()
				s.sampler = sampler
				s.lock.Unlock()
				s.metrics.SamplerUpdated.Inc(1)
			}
		} else {
			s.metrics.SamplerParsingFailure.Inc(1)
			s.logger.Infof("Unable to handle sampling strategy response %+v. Got error: %v", res, err)
		}
	} else {
		s.metrics.SamplerQueryFailure.Inc(1)
	}
}

func (s *remoteControlledSampler) extractSampler(res *sampling.SamplingStrategyResponse) (Sampler, error) {
	if probabilistic := res.GetProbabilisticSampling(); probabilistic != nil {
		sampler, err := NewProbabilisticSampler(probabilistic.SamplingRate)
		if err != nil {
			return nil, err
		}
		return sampler, nil
	}
	if rateLimiting := res.GetRateLimitingSampling(); rateLimiting != nil {
		sampler, err := NewRateLimitingSampler(float64(rateLimiting.MaxTracesPerSecond))
		if err != nil {
			return nil, err
		}
		return sampler, nil
	}
	return nil, fmt.Errorf("Unsupported sampling strategy type %v", res.GetStrategyType())
}
