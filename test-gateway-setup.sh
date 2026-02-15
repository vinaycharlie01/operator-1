# Create self-signed certificate
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /tmp/tls.key -out /tmp/tls.crt \
  -subj "/CN=158.176.9.206.nip.io"

# Create the secret
kubectl create secret tls envoy-gateway-myminio-tls -n tenant-ns \
  --cert=/tmp/tls.crt \
  --key=/tmp/tls.key

# Verify it was created
kubectl get secret -n tenant-ns envoy-gateway-myminio-tls

# Restart Envoy pods to pick up the secret
kubectl delete pods -n tenant-ns -l app=envoy-gateway,tenant=myminio

# Wait for pods to restart
kubectl get pods -n tenant-ns -l app=envoy-gateway,tenant=myminio -w



# Create wildcard certificate
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /tmp/tls.key \
  -out /tmp/tls.crt \
  -subj "/CN=*.158.176.9.206.nip.io" \
  -addext "subjectAltName=DNS:*.158.176.9.206.nip.io,DNS:158.176.9.206.nip.io"

# Verify the certificate
openssl x509 -in tmp/tls.key -text -noout | grep -A 2 "Subject Alternative Name"

# Delete old secret if exists
kubectl delete secret -n tenant-ns envoy-gateway-myminio-tls --ignore-not-found

# Create new wildcard secret
kubectl create secret tls envoy-gateway-myminio-tls -n tenant-ns \
  --cert=/tmp/tls.crt \
  --key=/tmp/tls.key

# Verify secret was created
kubectl get secret -n tenant-ns envoy-gateway-myminio-tls

# Restart Envoy pods to pick up new certificate
kubectl delete pods -n tenant-ns -l app=envoy-gateway,tenant=myminio

# Wait for pods to restart
kubectl get pods -n tenant-ns -l app=envoy-gateway,tenant=myminio -w
