package shared

const (
	ServerDefaultPort           = "16000"
	ServerDefaultListenAddress  = "0.0.0.0:" + ServerDefaultPort
	ClientDefaultConnectAddress = "localhost:" + ServerDefaultPort
	ServerDefaultCertsDir       = "./certs/server/"
	ClientDefaultCertsDir       = "./certs/client/"
	ServerDefaultCAFile         = "root_ca.pem"
	ServerDefaultCertFile       = "server.pem"
	ServerDefaultCertKeyFile    = "server.key"
	ClientDefaultCAFile         = "root_ca.pem"
	ClientDefaultCertFile       = "client.pem"
	ClientDefaultCertKeyFile    = "client.key"
)
