package jaeger

// RemotelyControlledSamplerOption is a function that sets some option on the sampler
type RemotelyControlledSamplerOption func(sampler *RemotelyControlledSampler)

// RemotelyControlledSamplerOptions is a factory for all available RemotelyControlledSamplerOption's
var RemotelyControlledSamplerOptions remotelyControlledSamplerOptions

type remotelyControlledSamplerOptions struct{}

// Metrics creates a RemotelyControlledSamplerOption that initializes Metrics on the sampler,
// which is used to emit statistics.
func (remotelyControlledSamplerOptions) Metrics(m *Metrics) RemotelyControlledSamplerOption {
	return func(s *RemotelyControlledSampler) {
		s.metrics = *m
	}
}

// MaxOperations creates a RemotelyControlledSamplerOption that sets the maximum number of
// operations the sampler will keep track of.
func (remotelyControlledSamplerOptions) MaxOperations(maxOperations int) RemotelyControlledSamplerOption {
	return func(s *RemotelyControlledSampler) {
		s.maxOperations = maxOperations
	}
}

// InitialSampler creates a RemotelyControlledSamplerOption that sets the initial sampler
// to use before a remote sampler is created and used.
func (remotelyControlledSamplerOptions) InitialSampler(sampler Sampler) RemotelyControlledSamplerOption {
	return func(s *RemotelyControlledSampler) {
		s.sampler = sampler
	}
}

// Logger creates a RemotelyControlledSamplerOption that sets the logger used by the sampler.
func (remotelyControlledSamplerOptions) Logger(logger Logger) RemotelyControlledSamplerOption {
	return func(s *RemotelyControlledSampler) {
		s.logger = logger
	}
}

// HostPort creates a RemotelyControlledSamplerOption that sets the host:port of the local
// agent that contains the sampling strategies.
func (remotelyControlledSamplerOptions) HostPort(hostPort string) RemotelyControlledSamplerOption {
	return func(s *RemotelyControlledSampler) {
		s.hostPort = hostPort
	}
}
