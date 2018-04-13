// Config is put into a different package to prevent cyclic imports in case
// it is needed in several locations

package config

type Config struct {
	Port string `config:"port"`
}

var DefaultConfig = Config{
	Port: "8081",
}
