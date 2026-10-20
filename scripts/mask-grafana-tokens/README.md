# Mask Grafana Tokens

Masks each Grafana service account token in the workflow logs when the `grafana_sa_token` input is a JSON object of tokens per instance. GitHub masks a secret as a whole, but not the individual tokens within it.

```bash
GRAFANA_SA_TOKEN='{"https://prod.example": "glsa_prod", "https://staging.example": "glsa_staging"}' \
  scripts/mask-grafana-tokens/mask-grafana-tokens.sh
```

A plain token, an empty value, or anything that isn't a JSON object is ignored, as GitHub already masks the secret.
