# Bulk-issuance test artifacts

## Quickstart

Start the two "ministry" containers (isolated registry + JSON API, no
impact on the main VC stack):

```bash
cd ~/cdpi/n8n-demo/verifiably-go/testdata/bulk-issuance
docker compose up -d
```

Expected:

```
[+] Running 2/2
 ✔ Container ministry-citizens-db       Healthy
 ✔ Container ministry-citizens-service  Running
```

Verify both containers are healthy and the service is answering:

```bash
docker compose ps
curl -s http://localhost:8199/health | jq
```

```
NAME                        STATUS                   PORTS
ministry-citizens-db        Up X minutes (healthy)   0.0.0.0:5437->5432/tcp
ministry-citizens-service   Up X minutes (healthy)   0.0.0.0:8199->8099/tcp

{
  "ok": true,
  "routes": [
    "/api/farmer-id",
    "/api/hotel-reservation",
    "/api/mortgage-eligibility",
    "/api/mortgage-simple",
    "/api/verifiable-id"
  ]
}
```

That's it — the **API** and **DB** sources in the VC platform's bulk-issue
form now have something to talk to. Jump to the recipes below to drive
each source. When you're finished:

```bash
docker compose down -v   # tears down both containers and drops the synthetic citizens volume
```

> **Ports cheat-sheet** — `5437` is postgres (DSN
> `postgres://citizens:citizens@localhost:5437/citizens` from the host,
> or `@ministry-citizens-db:5432` from inside docker). `8199` is the
> HTTP API (`http://localhost:8199/...` from the host, or
> `http://ministry-citizens-service:8099/...` from inside docker).

---

Three input sources are supported by the issuer's bulk form:

| chip | handler (bulk.go)      | what it reads                                    |
|------|------------------------|---------------------------------------------------|
| csv  | `SimulateCSV`          | multipart file upload                             |
| api  | `BulkFromAPI`          | `GET` returning a JSON array (or `{rows:[...]}`)  |
| db   | `BulkFromDB`           | postgres DSN + SELECT query                       |

Every source produces the same `[]map[string]string`, so the schema's
field-name → row-key mapping is what determines whether a row issues
cleanly. Field names are **case-sensitive** (camelCase, matching the
walt.id template the schema was built from).

Everything below is **dummy data** — 200 synthetic Kenyan + Trinidad &
Tobago citizens seeded into a throw-away postgres. Copy-paste the recipes
verbatim; nothing references real PII.

## Layout

```
testdata/bulk-issuance/
├── csv/                         # drop into the CSV source
│   ├── mortgage-simple.csv                 10 rows, single "holder" column
│   ├── mortgage-simple-large.csv           50 rows — scale / batching test
│   ├── mortgage-eligibility.csv            10 rows, all 10 walt.id MortgageEligibility fields
│   ├── verifiable-id.csv                   10 rows, 8 VerifiableId fields
│   ├── hotel-reservation.csv               10 rows, 5 HotelReservation fields
│   ├── tax-receipt.csv                     10 rows, 2 TaxReceipt fields
│   ├── malformed-bad-quoting.csv           unterminated quotes → parseCSVRows error
│   ├── malformed-wrong-columns.csv         column-count drift → per-row rejection
│   └── header-only-no-rows.csv             zero data rows → IssueBulk returns 0 accepted
├── db/
│   └── queries.sql              # paste-ready SELECTs, one per schema, aliasing
│                                  citizens-db columns to schema field names
├── api/
│   ├── citizen_service.py       # stdlib-only HTTP server wrapping citizens-db
│   ├── Dockerfile               # container image for the service
│   └── run.sh                   # bare-Python launcher (optional bearer auth)
└── docker-compose.yml           # "ministry" scenario: db + API in one stack
```

## Common setup (do once per test)

1. Open the VC platform (`http://172.24.0.1:8080` local, or
   `http://<host>:8080` remote).
2. **Issuer** role → log in (`admin` / `admin` via keycloak).
3. **Issuer → DPG**: pick **Walt Community Stack**, continue.
4. **Issuer → Schema**: pick the schema listed in the recipe below, then
   the **JWT · W3C** (`jwt_vc_json`) chip.
5. **Issuer → Mode**: select **Bulk** + **Wallet**, continue.
6. You're now on the issuance screen with three chips:
   **CSV upload**, **Secured API**, **Database**.

Swap step 4's schema pick to match whichever recipe you're running — the
bulk form only accepts rows whose keys match the selected schema's
`credentialSubject` fields.

---

## CSV source

Chip → **CSV upload** → upload the file → **Upload & issue**.

| DPG · Schema (step 3 + 4)                               | File                                | Expect          |
|---------------------------------------------------------|-------------------------------------|------------------|
| Walt · Mortgage Eligibility (jwt_vc_json)               | `csv/mortgage-simple.csv`           | 10 rows · 10 issued |
| Walt · Mortgage Eligibility (jwt_vc_json)               | `csv/mortgage-simple-large.csv`     | 50 rows · 50 issued |
| Walt · Mortgage Eligibility (jwt_vc_json)               | `csv/mortgage-eligibility.csv`      | 10 rows · 10 issued |
| Walt · Verifiable Id (jwt_vc_json)                      | `csv/verifiable-id.csv`             | 10 rows · 10 issued |
| Walt · Hotel Reservation (jwt_vc_json)                  | `csv/hotel-reservation.csv`         | 10 rows · 10 issued |
| Walt · Tax Receipt (jwt_vc_json)                        | `csv/tax-receipt.csv`               | 10 rows · 10 issued |
| **Inji Certify Pre-Auth · Farmer Credential (V2)**      | `csv/farmer-credential.csv`         | 10 rows · 10 issued |
| *any*                                                   | `csv/malformed-bad-quoting.csv`     | ✗ red error toast (parse failure) |
| Walt · Verifiable Id                                    | `csv/malformed-wrong-columns.csv`   | Mixed — per-row rejections visible in the table |
| Walt · Mortgage Eligibility                             | `csv/header-only-no-rows.csv`       | ✗ "no rows" error |

---

## Secured API source

> **Note:** The Secured API chip is **hidden** when the Issuer DPG is
> set to **Inji Certify · Pre-Auth**. Per [docs.inji.io](https://docs.inji.io/inji-certify/overview),
> Inji Certify's Data Provider Plugin currently supports PostgreSQL + CSV
> only; an "API" reference implementation is listed as a 2025 roadmap
> item. The verifiably-go UI reflects that by gating the chip via
> the DPG's `Kind:"bulk_source"` capabilities.

Chip → **Secured API** → paste the URL → leave auth header blank →
**Fetch & issue**.

Use the URL that matches where **the VC platform** is running, not where
your laptop is:

- **VC platform in docker on the same host** (usual): use the service
  name directly on the `waltid_default` network —
  `http://ministry-citizens-service:8099/...`
- **VC platform on bare metal** (`go run ./cmd/server`):
  `http://localhost:8199/...`
- **VC platform on a different machine** than the ministry:
  `http://<ministry-host>:8199/...`

All five endpoints, paste-ready (same-host-docker flavour):

| Schema pick (step 4)               | URL                                                                           | Expect |
|------------------------------------|-------------------------------------------------------------------------------|--------|
| Mortgage Eligibility (jwt_vc_json)   | `http://ministry-citizens-service:8099/api/mortgage-simple?limit=10`          | 10 rows · 10 issued |
| Mortgage Eligibility (jwt_vc_json)   | `http://ministry-citizens-service:8099/api/mortgage-eligibility?limit=10`     | 10 rows · 10 issued |
| Verifiable Id (jwt_vc_json)        | `http://ministry-citizens-service:8099/api/verifiable-id?limit=15`            | 15 rows · 15 issued |
| Hotel Reservation (jwt_vc_json)    | `http://ministry-citizens-service:8099/api/hotel-reservation?limit=10`        | 10 rows · 10 issued |
| *custom schema `{holder, farmId, ...}`* | `http://ministry-citizens-service:8099/api/farmer-id?limit=10`           | 10 rows · 10 issued |

**Bearer-auth variant** — bring the service up with a token:

```bash
cd testdata/bulk-issuance
docker compose down citizens-service
CITIZENS_API_TOKEN=hunter2 docker compose up -d citizens-service
```

Now the same URL with the auth field blank returns **HTTP 401** (the bulk
form shows the raw response), and with the field set to `Bearer hunter2`
it works again.

**Error-path URLs to paste** (same recipe, bad URLs):

| URL                                                             | Expect |
|-----------------------------------------------------------------|--------|
| `http://ministry-citizens-service:8099/api/does-not-exist`      | ✗ HTTP 404 in toast |
| `http://localhost:8199/api/mortgage-simple?limit=5`             | ✗ connection refused — wrong hostname (inside-docker `localhost` ≠ host) |
| stop the service, re-issue                                       | ✗ connection refused from the adapter |

---

## Database source

Chip → **Database** → paste the connection string + query → **Submit**.

### Connection strings

Pick the one that matches where the VC platform runs:

| VC platform location | DSN |
|---------------------|-----|
| **In docker on same host** (usual) | `postgres://citizens:citizens@ministry-citizens-db:5432/citizens` |
| Bare metal `go run` | `postgres://citizens:citizens@localhost:5437/citizens` |
| Different host | `postgres://citizens:citizens@<ministry-host>:5437/citizens` |

### Paste-ready queries (one per schema)

**Simple holder-only** — works with any one-field custom schema, or with
Mortgage Eligibility if you only care about the holder column:

```sql
SELECT first_name || ' ' || last_name AS holder
FROM citizens
ORDER BY id
LIMIT 10;
```

**MortgageEligibility** (walt.id catalog, 10 fields):

```sql
SELECT
  CASE gender WHEN 'Male' THEN 'Mr' WHEN 'Female' THEN 'Mrs' ELSE '' END AS salutation,
  first_name                       AS "firstName",
  last_name                        AS "familyName",
  email                            AS "emailAddress",
  date_of_birth::text              AS "dateOfBirth",
  (400000 + (id * 1750) % 500000)::text  AS "purchasePrice",
  (60000  + (id * 320)  % 120000)::text  AS "totalIncome",
  (320000 + (id * 1400) % 400000)::text  AS "mortgageAmount",
  CASE (id % 4) WHEN 0 THEN 'none' WHEN 1 THEN 'vehicle' WHEN 2 THEN 'savings' ELSE 'shares' END AS "additionalCollateral",
  LPAD((id * 37)::text, 5, '0')   AS "postCodeProperty"
FROM citizens
ORDER BY id
LIMIT 10;
```

**VerifiableId** (walt.id catalog, 8 fields):

```sql
SELECT
  first_name                       AS "firstName",
  last_name                        AS "familyName",
  date_of_birth::text              AS "dateOfBirth",
  gender                           AS gender,
  place_of_birth                   AS "placeOfBirth",
  address                          AS "currentAddress",
  national_id                      AS "personalIdentifier",
  first_name || ' ' || last_name   AS "nameAndFamilyNameAtBirth"
FROM citizens
WHERE address IS NOT NULL
ORDER BY id
LIMIT 15;
```

**HotelReservation** (walt.id catalog, 5 fields):

```sql
SELECT
  first_name                           AS "firstName",
  last_name                            AS "familyName",
  date_of_birth::text                  AS "dateOfBirth",
  place_of_birth                       AS "placeOfBirth",
  'Suite ' || (100 + id % 400)::text || ', Hotel Sample' AS "currentAddress"
FROM citizens
ORDER BY id
LIMIT 10;
```

**University degree** — only citizens who actually have a degree in the
seed data (match with a custom schema that has fields
`holder, degree, major, graduationDate, issuer, gpa`):

```sql
SELECT
  first_name || ' ' || last_name AS holder,
  degree_type                    AS degree,
  major                          AS major,
  graduation_date::text          AS "graduationDate",
  university                     AS issuer,
  COALESCE(gpa::text, '')        AS gpa
FROM citizens
WHERE university IS NOT NULL
ORDER BY id
LIMIT 15;
```

**Farmer ID** — only registered farmers (match with a custom schema with
fields `holder, farmId, location, hectares, crops, registeredOn`):

```sql
SELECT
  first_name || ' ' || last_name         AS holder,
  farm_id                                AS "farmId",
  farm_location                          AS location,
  COALESCE(farm_size_hectares::text, '') AS hectares,
  COALESCE(primary_crops, '')            AS crops,
  farm_registration_date::text           AS "registeredOn"
FROM citizens
WHERE farm_id IS NOT NULL
ORDER BY id
LIMIT 10;
```

**Inji Certify · Farmer Credential (V2)** — 13 fields matching the live
Inji Certify instance's `/v1/certify/.well-known/openid-credential-issuer`
`order` array. Only includes citizens registered as farmers. Pair with
the **Farmer Credential (V2)** schema in the Inji Certify Pre-Auth DPG:

```sql
SELECT
  first_name || ' ' || last_name                       AS "fullName",
  COALESCE(phone, '+254000000000')                     AS "mobileNumber",
  date_of_birth::text                                  AS "dateOfBirth",
  gender                                               AS gender,
  CASE country_code WHEN 'KE' THEN 'Kenya' WHEN 'TT' THEN 'Trinidad & Tobago' ELSE country_code END AS state,
  place_of_birth                                       AS district,
  split_part(farm_location, ' from ', 2)               AS "villageOrTown",
  '00100'                                              AS "postalCode",
  COALESCE(farm_size_hectares::text, '1.0')            AS "landArea",
  'owned'                                              AS "landOwnershipType",
  COALESCE(split_part(primary_crops, ',', 1), 'Maize') AS "primaryCropType",
  COALESCE(split_part(primary_crops, ',', 2), 'Beans') AS "secondaryCropType",
  farm_id                                              AS "farmerID"
FROM citizens
WHERE farm_id IS NOT NULL
ORDER BY id
LIMIT 10;
```

### Error-path queries

**Zero rows** — expect `✗ query returned 0 rows`:

```sql
SELECT first_name || ' ' || last_name AS holder
FROM citizens
WHERE country_code = 'ZZ';
```

**Non-SELECT** — blocked by bulk.go before it reaches postgres, expect
`✗ only SELECT queries allowed`:

```sql
DELETE FROM citizens WHERE id < 0;
```

**Bad DSN** — paste `postgres://citizens:wrong@ministry-citizens-db:5432/citizens`
with any valid query, expect `✗ password authentication failed`.

---

## After issuance — what each row gives you

The result screen shows a table: row #, recipient name, ✓/✗ status,
**full selectable offer URI**, and per-row **Copy link** / **QR** buttons.
Plus at the bottom:

- **Copy all offer links** — TSV (recipient ↹ offer URI) to the clipboard
- **Download CSV** — audit file with `row, recipient, status, offer_uri, error`

To verify a credential actually lands in a wallet:
1. Copy any offer URI from the table.
2. Sign out (top-right icon) or open a private browser window.
3. Log in as **Holder** (keycloak admin/admin). Pick Walt Community Stack.
4. **Holder → Wallet → Paste offer link**, paste, accept.
5. The held credential's claim values should match the row's data
   (`holder: Grace Atieno`, etc.).

---

## Scripted regression

From the `verifiably-go/` root:

```bash
BASE=http://172.24.0.1:8080 CSV=mortgage-simple.csv        node e2e/walkBulkCSV.mjs
BASE=http://172.24.0.1:8080 API='http://ministry-citizens-service:8099/api/mortgage-simple?limit=5' \
                                                            node e2e/walkBulkAPI.mjs
BASE=http://172.24.0.1:8080 CONN='postgres://citizens:citizens@ministry-citizens-db:5432/citizens' \
                            QUERY="SELECT first_name || ' ' || last_name AS holder FROM citizens ORDER BY id LIMIT 5" \
                                                            node e2e/walkBulkDB.mjs
BASE=http://172.24.0.1:8080                                 node e2e/assert-bulk-ui.mjs
```

All four scripts exit non-zero on regression.

## One-line row-count sanity

```bash
wc -l testdata/bulk-issuance/csv/*.csv
```
