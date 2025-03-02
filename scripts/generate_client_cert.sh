#!/bin/bash
set -e
# set -x
source "scripts/common.env"

# Check if the client directory is already present
[ -n "${CLIENT_CERT_DIR}" ] && [ -d "${CLIENT_CERT_DIR}" ] && echo "${CLIENT_CERT_DIR} already present" && exit 1
mkdir -p ${CLIENT_CERT_DIR}

# Generate the private key for the client certificate
${OPENSSL} ecparam -name ${CURVE} -outform PEM -genkey -out ${CLIENT_KEY}

# Generate client CSR
${OPENSSL} req -new -key ${CLIENT_KEY} -${DIGEST} \
    -out ${CLIENT_CSR} -config ${CLIENT_CONFIG_FILE}

# Sign CSR and generate client certificate
${OPENSSL} x509 -req -in ${CLIENT_CSR} \
    -CA ${ROOT_CERT} -CAkey ${ROOT_KEY} \
    -set_serial ${CLIENT_SERIAL_NUM} -days ${CLIENT_DAYS_VALID} -${DIGEST} \
    -extensions v3_ca -extfile ${CLIENT_CONFIG_FILE} \
    -out ${CLIENT_CERT} 2> /dev/null

# Copy root certificate so that the
# client can verify server
cp ${ROOT_CERT} ${CLIENT_CERT_DIR}
echo "Client certificate and private key created successfully:"
echo " - Private Key: ${CLIENT_KEY}"
echo " - Certificate: ${CLIENT_CERT}"
