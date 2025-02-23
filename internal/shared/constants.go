package shared

const (
	ServerDefaultPort           = "16000"
	ServerDefaultListenAddress  = "0.0.0.0:" + ServerDefaultPort
	ClientDefaultConnectAddress = "localhost:" + ServerDefaultPort
	ServerDefaultCertsDir       = "./certs/server/"
	ClientDefaultCertsDir       = "./certs/client/"
	DefaultCAFile               = "root_ca.pem"
	DefaultCertFile             = "server.pem"
	DefaultCertKeyFile          = "server.key"
)
