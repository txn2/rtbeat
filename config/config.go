// Config is put into a different package to prevent cyclic imports in case
// it is needed in several locations

package config

type Config struct {
	Port string `config:"port"`
	// Timeout is the per-request budget, in seconds, to wait for the output
	// to acknowledge a batch before POST /in responds 504. Durability hinges
	// on this: the 200 means "delivered", not merely "received".
	Timeout int `config:"timeout"`
	// ShutdownTimeout is the maximum time, in seconds, Stop() waits for
	// in-flight events to be acknowledged before closing the publisher.
	ShutdownTimeout int `config:"shutdown_timeout"`
}

var DefaultConfig = Config{
	Port:            "8081",
	Timeout:         5,
	ShutdownTimeout: 30,
}
