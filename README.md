## Setup
```
cp ./env.example ./.env
```
```
docker run --name todo-app-postgres -e POSTGRES_PASSWORD=mysecretpassword -d -p 5432:5432 postgres
```
Look at `setup.sql` for how to create tables

## Run the thing

```
go mod tidy
go run .
```
