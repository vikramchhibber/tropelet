package shared

const (
	ServerDefaultPort          = "16000"
	ServerDefaultListenAddress = "0.0.0.0:" + ServerDefaultPort
	ServerDefaultCertsDir      = "./certs/server/"
	ServerDefaultCAFile        = "root_ca.pem"
	ServerDefaultCertFile      = "server.pem"
	ServerDefaultCertKeyFile   = "server.key"

	ClientDefaultConnectAddress = "localhost:" + ServerDefaultPort
)
