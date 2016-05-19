enum SamplingStrategyType { PROBABILISTIC, RATE_LIMITING }

// ProbabilisticSamplingStrategy randomly samples a fixed percentage of all traces.
struct ProbabilisticSamplingStrategy {
    1: required double samplingRate // percentage expressed as rate (0..1]
}

// RateLimitingStrategy samples traces with a rate that does not exceed specified number of traces per second.
// The recommended implementation approach is leaky bucket.
struct RateLimitingSamplingStrategy {
    1: required i16 maxTracesPerSecond
}

struct SamplingStrategyResponse {
    1: required SamplingStrategyType strategyType
    2: optional ProbabilisticSamplingStrategy probabilisticSampling
    3: optional RateLimitingSamplingStrategy rateLimitingSampling
}

service SamplingManager {
    SamplingStrategyResponse getSamplingStrategy(1: string serviceName)
}
