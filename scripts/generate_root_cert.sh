#!/bin/bash
set -e
# set -x
source "scripts/common.env"

# Re-initialize root-ca files
[ -n "${ROOT_CERT_DIR}" ] && [ -d "${ROOT_CERT_DIR}" ] && rm -rf "${ROOT_CERT_DIR}"
mkdir -p ${ROOT_CERT_DIR}

# Generate the private key for the root certificate
${OPENSSL} ecparam -name ${CURVE} -outform PEM -genkey -out ${ROOT_KEY}

# Generate the root certificate
${OPENSSL} req -x509 -new -nodes -key ${ROOT_KEY} -${DIGEST} \
    -days ${ROOT_DAYS_VALID} -out ${ROOT_CERT} -set_serial ${ROOT_SERIAL_NUM} \
    -config ${ROOT_CONFIG_FILE}

echo "Root certificate and private key created successfully:"
echo " - Private Key: ${ROOT_KEY}"
echo " - Certificate: ${ROOT_CERT}"
