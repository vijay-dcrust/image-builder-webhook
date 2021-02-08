name=$1
#echo $name
#Create the private key
openssl genrsa -out ${name}.key 4096
# Create the certificate signing request
openssl req -new -key ${name}.key  -out ${name}.csr -config config.txt
# Create the ca key
openssl genrsa -out ca.key 4096
# create the ca request
openssl req -new -x509 -key ca.key -out ca.crt
#Enable the v3 extention with -extfile option with opnssl
# Enable the subjectAlterNativeName with -extfile option
openssl x509 -req -in ${name}.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out ${name}.crt -extfile v3.ext
cat ca.srl

