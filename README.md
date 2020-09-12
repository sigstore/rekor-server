# Rekor Server

Rekór - Greek for “Record”

Rekor's goals are to provide an immutable tamper resistant ledger of metadata generated within a software project or supply chain.  Rekor would enable software maintainers and build systems to submit signed digests to an immutable record. Other parties can then query said metadata to enable them to make informed decisions on trust and nonrepudiation of an object's lifecycle, based on signed metadata stored within a tamper proof binary (merkle) tree.

Rekor seeks to provide provenance and integrity of the software supply chain.

## Early Development / Experimental use only.

## Create Database and populate tables

Trillian requires a database, we use MariaDB in this instance. Once this
is installed on your machine edit the `scripts/createdb.sh` with your
database root account credentials and run the script.

## Build Trillian

To run rekor you need to build trillian

```
go get github.com/google/trillian.git
go build ./cmd/trillian_log_server
go build ./cmd/trillian_log_signer
go build ./cmd/createtree/

```

### Start the tlog server

```
trillian_log_server -http_endpoint=localhost:8090 -rpc_endpoint=localhost:8091 --logtostderr ...
```

### Start the tlog signer

```
trillian_log_signer --logtostderr --force_master --http_endpoint=localhost:8190 -rpc_endpoint=localhost:8191  --batch_size=1000 --sequencer_guard_window=0 --sequencer_interval=200ms
```

## Create a tree (note the return value)

```
./createtree --admin_server=localhost:8091 > logid
cat logid
2587331608088442751
```

## Add tlog_id to the rekor-server.yaml

```
trillian_log_server:
  address: "127.0.0.1"
  port: 8091
  tlog_id: 2587331608088442751
```

## Build Rekor Server

`go build`

## Start the rekor server

```
./rekor-server serve
2020-09-12T16:32:22.705+0100	INFO	cmd/root.go:87	Using config file: /Users/lukehinds/go/src/github.com/projectrekor/rekor-server/rekor-server.yaml
2020-09-12T16:32:22.705+0100	INFO	app/server.go:55	Starting server...
2020-09-12T16:32:22.705+0100	INFO	app/server.go:61	Listening on 127.0.0.1:3000
```

## Add an entry

```
echo > hello-rekor > /tmp/file.txt

curl http://localhost:3000/api/v1/add -F "fileupload=@/tmp/file.txt" -v
```

## Get an entry

```
curl http://localhost:3000/apt/v1/get -F "fileupload=@/tmp/file.txt" -v
```