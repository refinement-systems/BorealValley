# Setup

Safari on macOS uses the system trust store (Keychain), so if you create a local CA and mark its root certificate as trusted for TLS, certificates you issue from it will validate normally in Safari (no interstitial warnings), as long as the hostname matches the certificate’s SAN.  ￼

A practical setup (recommended): mkcert (fastest)
	1.	Install and create a local CA, and install it into the macOS trust store:

	•	brew install mkcert
	•	mkcert -install (creates a local CA and installs it in the system root store).  ￼

	2.	Generate a leaf certificate for the exact names you will use:

	•	mkcert example.test localhost 127.0.0.1 ::1
This produces a cert/key you point your HTTPS server at. (Make sure your browser URL matches one of these names, and that the cert contains them as SANs.)

	3.	Configure your backend/reverse proxy to use that cert/key (nginx/Caddy/your Go server/etc.), then open https://example.test in Safari.

Manual trust / troubleshooting on macOS
If you’re installing a root CA certificate yourself (or via a configuration profile), you may need to explicitly enable trust in Keychain Access: import/add the cert to the System or login keychain, then set Trust → “Always Trust” (or at least “When using this certificate” for SSL).  ￼

More production-like local CA (recommended if you want a “real” internal PKI)
Use something like Smallstep’s step-ca to run a private CA and issue short-lived certs, intermediate CAs, ACME, etc., then trust the root in macOS Keychain the same way.  ￼

Common gotchas
	•	Hostname mismatch: Safari won’t accept CN-only; the hostname must be in SAN.
	•	HSTS: if you previously visited a domain with HSTS, you can get stuck with stricter behavior; use a fresh dev-only domain (e.g., *.test) and map it in /etc/hosts.
	•	macOS 13+: roots installed via profiles may not be trusted for TLS by default unless you enable trust in Keychain Access.  ￼

# Revoke

In a local-dev CA setup, “revocation” only matters if your clients actually check revocation (CRL/OCSP) and your CA publishes it. Many local setups (including mkcert) don’t provide meaningful revocation for leaf certs; the practical way to invalidate “all old certs” is to rotate the CA (remove/distrust the old root, create a new root, reissue fresh leaf certs).

If you were using mkcert
	1.	Remove trust of the old mkcert root CA (this effectively invalidates every leaf cert issued from it in Safari).

	•	mkcert -uninstall (removes the mkcert root from system/browser trust).  ￼

	2.	Remove the old CA files (optional but recommended, since you lost the leaf certs anyway).

	•	rm -rf "$(mkcert -CAROOT)"  ￼

	3.	Create/install a new local CA and reissue new leaf certs.

	•	mkcert -install
	•	mkcert yourhost.test localhost 127.0.0.1 ::1  ￼

If you installed a root CA manually (Keychain) and want to delete it
You can remove/distrust it in Keychain Access, or delete it via security:
	•	sudo security delete-certificate -c "CERT_COMMON_NAME" /Library/Keychains/System.keychain  ￼
(Adjust the common name to match the CA cert’s CN.)

If you were running a “real” local CA (step-ca / Smallstep) and want actual revocation
Revocation is done by serial number (or by presenting the cert+key). Without the certificate file, you generally need the serial number (from logs/DB/issued-cert records) to revoke. The CLI supports revoking by serial number.  ￼

If you truly lost both the leaf cert files and any issuance records/serials, you can’t reliably revoke specific leaf certs; rotate the CA instead (new root/intermediate, and remove trust in the old one).

Net effect for Safari on macOS
Safari will stop trusting the old certificates as soon as the old CA root is no longer trusted (removed/distrusted). That’s the cleanest “revocation” mechanism for local testing.