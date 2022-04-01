#!/bin/bash

echo "keyspace set to ${DB_KEYSPACE}"

if [[ ! -z "$DB_KEYSPACE" && $1 = 'cassandra' ]]; then
  CQL="CREATE KEYSPACE IF NOT EXISTS $DB_KEYSPACE WITH REPLICATION = {'class': 'SimpleStrategy', 'replication_factor': 1};"
  until echo $CQL | cqlsh; do
    echo "post hook: waiting cassandra to be available"
    sleep 2
  done &
fi

exec docker-entrypoint.sh "$@"
