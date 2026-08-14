# REST-API-Tests mit Bruno

Die Collection in `tests/api/bruno/` wird aus der OpenAPI-SSOT unter
`docs/openapi.yaml` erzeugt. Sie testet die lokale REST-API von `symmemory serve`
und enthält keine echten Tokens oder Memory-Daten.

## Lokalen Daemon starten

```sh
symmemory serve -p 8787
```

Der öffentliche Status-Check benötigt kein Token:

```sh
cd tests/api/bruno
bru run 'api/status/Get server health and status.bru' \
  --env-file environments/local.bru
```

Geschützte Requests erhalten ihr JWT ausschließlich zur Laufzeit:

```sh
cd tests/api/bruno
bru run 'api/search/Semantic vector similarity search.bru' \
  --env-file environments/local.bru \
  --env-var token="$BRUNO_TOKEN"
```

`BRUNO_TOKEN` wird nicht in Git gespeichert. Schreibende oder löschende
Requests nur mit absichtlich angelegten Testdaten ausführen.

## Collection regenerieren

```sh
rm -rf tests/api/bruno
mkdir -p tests/api
bru import openapi \
  --source docs/openapi.yaml \
  --output tests/api/bruno \
  --collection-name "Symaira Memory REST API" \
  --collection-format bru \
  --group-by path
```

Danach `environments/local.bru` und diese README bei Bedarf wiederherstellen
und den Diff prüfen.
