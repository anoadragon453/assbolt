# Assbolt

A simple, local GUI for the host [Passbolt](https://www.passbolt.com/) password manager.

The motivation for this project was to have a desktop version of Passbolt working on Linux environments.

## Build

Enter the [nix](https://nixos.org/) development environment:

```
nix develop .
```

Build and run (Wayland):

```
go run -tags wayland .
```

or on X11:

```
go run .
```

## Configure

Passbolt details are configured as environment variables. It's recommended to
create a `.env` file with these entries:

```
PASSBOLT_URL=https://your.passbolt.example
PASSBOLT_PASSPHRASE=your-gpg-passphrase

# choose one of:
PASSBOLT_PRIVATE_KEY="-----BEGIN PGP PRIVATE KEY BLOCK----- …"
# --OR--
PASSBOLT_PRIVATE_KEY_FILE=/path/to/private.key

PASSBOLT_TOTP_SECRET=ABCDEF1234567890   # Base-32 secret from your authenticator app
```

If `PASSBOLT_TOTP_SECRET` is not set and Multi-Factor Authentication (MFA) is
enabled on your Passbolt account, you will be prompted for a six-digit TOTP
code on every startup.

> [!WARNING]
> It's strongly advised to not put all necessary secrets to log in to
> your Passbolt account in an unencrypted file on disk.

To retrieve your passbolt private key file, go to
https://<YOUR_PASSBOLT_URL>/app/settings/keys and click on Download Private Key
at the bottom.
