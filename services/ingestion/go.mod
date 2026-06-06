module github.com/sre-oncall/ingestion

go 1.26.4

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/jackc/pgx/v5 v5.6.0
	github.com/redis/go-redis/v9 v9.6.1
	github.com/sre-oncall/pkg v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/golang-migrate/migrate/v4 v4.17.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/crypto v0.20.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace github.com/sre-oncall/pkg => ../../pkg
