# Citizens Database — Mock Government Registry

A Postgres 16 instance with 200 seeded citizen records. Powers the
data-source layer for the v1 demo and represents the kind of system real
governments would integrate against (a national identity registry, a Sunbird
RC instance, a CSV dump from a ministry, etc.).

## Distribution

- 100 Kenyan citizens (`country_code='KE'`)
- 100 Trinidad & Tobago citizens (`country_code='TT'`)
- ~30% have university degree records
- ~40% have farmer ID records
- All 200 have birth registration records

## Connection

| Setting  | Value           |
| -------- | --------------- |
| Host     | `localhost`     |
| Port     | `5435`          |
| Database | `citizens`      |
| User     | `citizens`      |
| Password | `citizens`      |

From inside the docker network:

| Setting | Value             |
| ------- | ----------------- |
| Host    | `citizens-postgres` |
| Port    | `5432`            |

## Schema

```sql
SELECT * FROM citizens LIMIT 1;
```

| Column                       | Type         | Notes                                        |
| ---------------------------- | ------------ | -------------------------------------------- |
| `id`                         | SERIAL       | Primary key                                  |
| `national_id`                | VARCHAR(30)  | Unique national identifier                   |
| `country_code`               | CHAR(2)      | `KE` or `TT`                                 |
| `first_name`, `middle_name`, `last_name` | VARCHAR | Personal name                                |
| `gender`                     | VARCHAR(10)  | Male/Female                                  |
| `date_of_birth`              | DATE         |                                              |
| `place_of_birth`             | VARCHAR(200) | City/town                                    |
| `nationality`                | VARCHAR(50)  |                                              |
| `address`, `phone`, `email`  |              | Contact info                                 |
| `birth_registration_number`  | VARCHAR(50)  | Used for **Birth Certificate** credential    |
| `birth_registration_date`    | DATE         |                                              |
| `mother_name`, `father_name` |              | Used for **Birth Certificate** credential    |
| `university`                 | VARCHAR      | Used for **University Degree** credential    |
| `degree_type`                | VARCHAR(50)  | BSc, BA, MSc, etc.                           |
| `major`                      | VARCHAR(150) |                                              |
| `graduation_date`            | DATE         |                                              |
| `gpa`                        | DECIMAL(3,2) |                                              |
| `student_id`                 | VARCHAR(50)  |                                              |
| `farm_id`                    | VARCHAR(30)  | Used for **Farmer ID** credential            |
| `farm_location`              | VARCHAR(300) |                                              |
| `farm_size_hectares`         | DECIMAL      |                                              |
| `primary_crops`              | VARCHAR(500) | Comma-separated                              |
| `farm_registration_date`     | DATE         |                                              |

## Sample Queries

### Find a citizen by national ID
```sql
SELECT * FROM citizens WHERE national_id = 'KE-NID-12345678';
```

### List Kenyan farmers
```sql
SELECT national_id, first_name, last_name, farm_id, farm_size_hectares, primary_crops
FROM citizens
WHERE country_code = 'KE' AND farm_id IS NOT NULL
ORDER BY farm_size_hectares DESC
LIMIT 20;
```

### List Trinidadian university graduates
```sql
SELECT national_id, first_name, last_name, university, degree_type, major, gpa
FROM citizens
WHERE country_code = 'TT' AND university IS NOT NULL
ORDER BY graduation_date DESC
LIMIT 20;
```

## Credential Type → Column Mapping

The data source plugin maps these columns to credential subject claims.

### University Degree
```
name             ← first_name || ' ' || last_name
holderName       ← first_name || ' ' || last_name
nationalId       ← national_id
institution      ← university
degree           ← degree_type
major            ← major
graduationDate   ← graduation_date
gpa              ← gpa
studentId        ← student_id
```

### Farmer ID
```
fullName         ← first_name || ' ' || last_name
mobileNumber     ← phone
dateOfBirth      ← date_of_birth
gender           ← gender
state            ← country (KE → Kenya, TT → Trinidad and Tobago)
district         ← place_of_birth
villageOrTown    ← place_of_birth
postalCode       ← (extracted from address)
landArea         ← farm_size_hectares
landOwnershipType ← 'Owned'  -- default
primaryCropType  ← (first crop in primary_crops)
secondaryCropType ← (second crop in primary_crops)
farmerID         ← farm_id
```

### Birth Certificate
```
holderName            ← first_name || ' ' || (middle_name + ' ') || last_name
nationalId            ← national_id
dateOfBirth           ← date_of_birth
placeOfBirth          ← place_of_birth
gender                ← gender
nationality           ← nationality
motherName            ← mother_name
fatherName            ← father_name
registrationNumber    ← birth_registration_number
registrationDate      ← birth_registration_date
```

## Reseeding

The init.sql is idempotent — `DROP TABLE IF EXISTS citizens;` runs first.
On a fresh container start, the postgres entrypoint runs init.sql automatically.
To reseed an existing container without restarting:

```bash
docker exec -i citizens-postgres psql -U citizens -d citizens \
  < docker/waltid/citizens-db/init.sql
```
