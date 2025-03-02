#!/bin/bash
set -e
# set -x
source "scripts/common.env"

# Re-initialize server-ca files
[ -n "${SERVER_CERT_DIR}" ] && [ -d "${SERVER_CERT_DIR}" ] && rm -rf "${SERVER_CERT_DIR}"
mkdir -p ${SERVER_CERT_DIR}

# Generate the private key for the server certificate
${OPENSSL} ecparam -name ${CURVE} -outform PEM -genkey -out ${SERVER_KEY}

# Generate server CSR
${OPENSSL} req -new -key ${SERVER_KEY} -${DIGEST} \
    -out ${SERVER_CSR} -config ${SERVER_CONFIG_FILE} -extensions req_ext

# Sign CSR and generate server certificate
${OPENSSL} x509 -req -in ${SERVER_CSR} \
    -CA ${ROOT_CERT} -CAkey ${ROOT_KEY} \
    -set_serial ${SERVER_SERIAL_NUM} -days ${SERVER_DAYS_VALID} -${DIGEST} \
    -extensions v3_ca -extfile ${SERVER_CONFIG_FILE} \
    -out ${SERVER_CERT} 2> /dev/null

# Copy root certificate so that the
# server can verify client
cp ${ROOT_CERT} ${SERVER_CERT_DIR}
echo "Server certificate and private key created successfully:"
echo " - Private Key: ${SERVER_KEY}"
echo " - Certificate: ${SERVER_CERT}"
