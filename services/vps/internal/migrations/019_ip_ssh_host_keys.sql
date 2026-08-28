CREATE TABLE IF NOT EXISTS vps.ip_ssh_host_keys (
    ip               inet PRIMARY KEY,
    ed25519_private  text NOT NULL,
    ed25519_public   text NOT NULL,
    ecdsa_private    text,
    ecdsa_public     text,
    updated_at       timestamptz NOT NULL DEFAULT now()
);
