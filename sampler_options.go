package jaeger

// SamplerOption is a function that sets some option on the sampler
type SamplerOption func(options *samplerOptions)

// SamplerOptions is a factory for all available SamplerOption's
var SamplerOptions samplerOptions

type samplerOptions struct {
	metrics       *Metrics
	maxOperations int
	sampler       Sampler
	logger        Logger
	hostPort      string
}

// Metrics creates a SamplerOption that initializes Metrics on the sampler,
// which is used to emit statistics.
func (samplerOptions) Metrics(m *Metrics) SamplerOption {
	return func(o *samplerOptions) {
		o.metrics = m
	}
}

// MaxOperations creates a SamplerOption that sets the maximum number of
// operations the sampler will keep track of.
func (samplerOptions) MaxOperations(maxOperations int) SamplerOption {
	return func(o *samplerOptions) {
		o.maxOperations = maxOperations
	}
}

// InitialSampler creates a SamplerOption that sets the initial sampler
// to use before a remote sampler is created and used.
func (samplerOptions) InitialSampler(sampler Sampler) SamplerOption {
	return func(o *samplerOptions) {
		o.sampler = sampler
	}
}

// Logger creates a SamplerOption that sets the logger used by the sampler.
func (samplerOptions) Logger(logger Logger) SamplerOption {
	return func(o *samplerOptions) {
		o.logger = logger
	}
}

// HostPort creates a SamplerOption that sets the host:port of the local
// agent that contains the sampling strategies.
func (samplerOptions) HostPort(hostPort string) SamplerOption {
	return func(o *samplerOptions) {
		o.hostPort = hostPort
	}
}
