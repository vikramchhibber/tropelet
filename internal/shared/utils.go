package shared

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger = *zap.SugaredLogger

func CreateLogger() Logger {
	// Logger configuration to include time, level and message
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:    "time",
		LevelKey:   "level",
		MessageKey: "msg",
		LineEnding: zapcore.DefaultLineEnding,
		// Colored output for log level in upper case
		EncodeLevel: zapcore.CapitalColorLevelEncoder,
		EncodeTime:  zapcore.ISO8601TimeEncoder,
	}

	// Create a Core that writes logs to the console
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		// TODO: Candidate for config
		zap.NewAtomicLevelAt(zap.DebugLevel),
	)

	// Build the logger with this core
	return zap.New(core).Sugar()
}

func LoadCertificates(caBundlePath, certPath,
	certKeyPath string) (*x509.CertPool, *tls.Certificate, error) {
	var err error
	var pemContents = make([][]byte, 3)
	for i, path := range []string{caBundlePath, certPath, certKeyPath} {
		if pemContents[i], err = ReadFile(path); err != nil {
			return nil, nil, err
		}
	}

	// Load certificate and key pair
	certKeyPair, err := tls.X509KeyPair(pemContents[1], pemContents[2])
	if err != nil {
		return nil, nil, fmt.Errorf("error loading server certificates: %w", err)
	}

	// Load CA bundle for validating remote certificate
	certs, err := ParseCertificates(pemContents[0])
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA bundle: %w", err)
	}
	caPool := x509.NewCertPool()
	for _, cert := range certs {
		caPool.AddCert(cert)
	}

	return caPool, &certKeyPair, nil
}

func ParseCertificates(certificate []byte) ([]*x509.Certificate, error) {
	// Assume here the blocks start with "BEGIN CERTIFICATE".
	// There could be multiple of these blocks.
	certificates := make([]*x509.Certificate, 0)
	// Parse all the blocks
	var rest []byte
	rest = []byte(strings.TrimSpace(string(certificate)))
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil || rest == nil {
			break
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, cert)
	}
	if len(certificates) == 0 {
		return nil, fmt.Errorf("no certificate found")
	}

	return certificates, nil
}

func ReadFile(path string) ([]byte, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed reading %s: %w", path, err)
	}

	return content, err
}
