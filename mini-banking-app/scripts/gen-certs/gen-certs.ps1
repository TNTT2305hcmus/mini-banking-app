<#
.SYNOPSIS
  Generate the gRPC transport TLS material for the mini-banking stack (Windows twin of gen-certs.sh).

.DESCRIPTION
  Creates one internal transport CA and signs a server certificate for the CA, KDC
  and Bank services, then copies each file to the path its service loads at runtime.
  These are gRPC *transport* certs, unrelated to the application-level Root CA the
  CA service uses to issue client certificates (certs/root-ca).

  Requires openssl on PATH (OpenSSL 1.1.1+ / 3.x).

.PARAMETER Days
  Validity in days for the server certs (default 825). The CA cert lives 4x longer.

.EXAMPLE
  ./gen-certs.ps1
  ./gen-certs.ps1 -Days 90
#>
param(
    [int]$Days = 825
)

$ErrorActionPreference = "Stop"

$ScriptDir = $PSScriptRoot
$Root      = (Resolve-Path (Join-Path $ScriptDir "..\..")).Path
$Out       = Join-Path $ScriptDir "out"
New-Item -ItemType Directory -Force -Path $Out | Out-Null

$CaCrt = Join-Path $Out "grpc-ca.crt"
$CaKey = Join-Path $Out "grpc-ca.key"

Write-Host "==> Generating internal gRPC transport CA"
if (-not (Test-Path $CaKey)) {
    & openssl genrsa -out $CaKey 4096
    & openssl req -x509 -new -nodes -key $CaKey -sha256 -days ($Days * 4) `
        -subj "/C=VN/O=Mini_App_Banking/CN=Mini_App_Banking Internal gRPC CA" `
        -out $CaCrt
    Write-Host "    created CA"
} else {
    Write-Host "    reusing existing CA ($CaCrt)"
}

function New-ServerCert {
    param([string]$Name, [string]$Cn, [string]$Sans, [string]$DestCrt, [string]$DestKey)

    Write-Host "==> $Name server certificate (CN=$Cn)"
    $tmp = Join-Path $env:TEMP ("gencert-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    try {
        $key = Join-Path $tmp "srv.key"
        $csr = Join-Path $tmp "srv.csr"
        $crt = Join-Path $tmp "srv.crt"
        $ext = Join-Path $tmp "ext.cnf"

        & openssl genrsa -out $key 2048
        & openssl req -new -key $key -subj "/C=VN/O=Mini_App_Banking/CN=$Cn" -out $csr

        "subjectAltName=$Sans`nextendedKeyUsage=serverAuth`nkeyUsage=digitalSignature,keyEncipherment" |
            Out-File -FilePath $ext -Encoding ascii

        & openssl x509 -req -in $csr `
            -CA $CaCrt -CAkey $CaKey -CAcreateserial `
            -days $Days -sha256 -extfile $ext -out $crt

        New-Item -ItemType Directory -Force -Path (Split-Path $DestCrt) | Out-Null
        New-Item -ItemType Directory -Force -Path (Split-Path $DestKey) | Out-Null
        Copy-Item $crt $DestCrt -Force
        Copy-Item $key $DestKey -Force
        Write-Host "    -> $DestCrt"
    }
    finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
}

New-ServerCert -Name "CA"   -Cn "ca-service" `
    -Sans "DNS:ca-service,DNS:localhost,IP:127.0.0.1" `
    -DestCrt (Join-Path $Root "ca-service\certs\grpc\ca-server.crt") `
    -DestKey (Join-Path $Root "ca-service\certs\grpc\ca-server.key")

New-ServerCert -Name "KDC"  -Cn "kdc-service" `
    -Sans "DNS:kdc-service,DNS:localhost,IP:127.0.0.1" `
    -DestCrt (Join-Path $Root "kdc-service\certs\kdc-server.crt") `
    -DestKey (Join-Path $Root "kdc-service\certs\kdc-server.key")

New-ServerCert -Name "Bank" -Cn "banking-service" `
    -Sans "DNS:banking-service,DNS:bank-service,DNS:localhost,IP:127.0.0.1" `
    -DestCrt (Join-Path $Root "banking-service\certs\grpc\bank-server.crt") `
    -DestKey (Join-Path $Root "banking-service\certs\grpc\bank-server.key")

Write-Host "==> Distributing transport CA cert to verifiers"
function Copy-CaCert([string]$Dest) {
    New-Item -ItemType Directory -Force -Path (Split-Path $Dest) | Out-Null
    Copy-Item $CaCrt $Dest -Force
    Write-Host "    -> $Dest"
}
Copy-CaCert (Join-Path $Root "api-gateway\certs\grpc-ca.crt")
Copy-CaCert (Join-Path $Root "kdc-service\certs\grpc-ca.crt")
Copy-CaCert (Join-Path $Root "kdc-service\certs\grpc\ca-server-ca.crt")
Copy-CaCert (Join-Path $Root "banking-service\certs\grpc-ca.crt")

Write-Host ""
Write-Host "Done. Certs valid for $Days days. CA private key kept only in $CaKey."
Write-Host "Set the gateway CA_CERT_PATH to api-gateway/certs/grpc-ca.crt (or your deploy path)."
