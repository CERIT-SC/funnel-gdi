#!/usr/bin/env bash
# Acceptance scenarios beyond the TES compliance suite, run against a Funnel
# binary built with coverage (make build-cover):
#  1. the CLI (funnel run, task, storage, examples) against a server with
#     local compute, and the terminal dashboard (funnel dashboard)
#  2. scheduler mode: a server with manual compute and a node (funnel node run)
#  3. the Slurm, PBS, Grid Engine and HTCondor backends with fake scheduler
#     commands that run the job script locally
#  4. basic authentication and task access (OwnerOrAdmin)
#  5. a standalone worker (task file, base64 task, scratch directory)
# Every Funnel process writes its coverage to GOCOVERDIR. Optional: S3 storage
# (make start-s3) and FTP storage (make start-ftp) when their containers run.
#
# Usage: tests/acceptance/scenarios.sh <funnel binary> <coverage dir>
set -euo pipefail
# The CLI reads tasks from stdin when it is not a terminal.
exec < /dev/null

funnel=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
mkdir -p "$2"
export GOCOVERDIR=$(cd "$2" && pwd)
src=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/funnel-scenarios.XXXX")
cd "$work"
echo "Work directory: $work"

pids=()
cleanup() {
  for pid in "${pids[@]}"; do stop "$pid"; done
  [ -z "${http:-}" ] || kill "$http" 2>/dev/null || true
}
trap cleanup EXIT

# stop <pid>: SIGINT, so that Funnel writes its coverage data.
stop() {
  kill -INT "$1" 2>/dev/null || return 0
  for _ in $(seq 1 30); do kill -0 "$1" 2>/dev/null || return 0; sleep 1; done
  echo "process $1 did not stop; no coverage data"
  kill -KILL "$1" 2>/dev/null || true
}

# server <name> <http port> <rpc port> [config lines...]: starts a server;
# the arrays server_lines and client_lines add lines to its Server and
# RPCClient sections.
server_lines=()
client_lines=()
server() {
  local name=$1 http=$2 rpc=$3
  shift 3
  mkdir -p "$work/$name"
  {
    echo "Server:"
    echo "  HostName: localhost"
    echo "  HTTPPort: \"$http\""
    echo "  RPCPort: \"$rpc\""
    [ ${#server_lines[@]} -eq 0 ] || printf "%s\n" "${server_lines[@]}"
    echo "RPCClient:"
    echo "  ServerAddress: localhost:$rpc"
    [ ${#client_lines[@]} -eq 0 ] || printf "%s\n" "${client_lines[@]}"
    echo "Database: boltdb"
    echo "BoltDB:"
    echo "  Path: $work/$name/funnel.db"
    echo "Worker:"
    echo "  WorkDir: $work/$name/work"
    echo "LocalStorage:"
    echo "  AllowedDirs: [\"$work\"]"
    printf '%s\n' "$@"
  } > "$work/$name.yml"
  "$funnel" server run --config "$work/$name.yml" > "$work/$name.log" 2>&1 &
  pids+=($!)
  for _ in $(seq 1 30); do
    curl -sf "http://localhost:$http/healthz" > /dev/null && return 0
    sleep 1
  done
  echo "server $name did not start:"; cat "$work/$name.log"; exit 1
}

# expect <description> <command...>: runs the command, which must succeed.
expect() {
  local what=$1; shift
  if "$@" > "$work/out" 2>&1; then echo "ok   $what"; else echo "FAIL $what"; cat "$work/out"; exit 1; fi
}

# expect_fail <description> <command...>: the command must fail.
expect_fail() {
  local what=$1; shift
  if "$@" > "$work/out" 2>&1; then echo "FAIL $what (succeeded)"; cat "$work/out"; exit 1; else echo "ok   $what"; fi
}

state() { "$funnel" task get -S "$1" --view MINIMAL "$2" | jq -r .state; }

echo "== 1. CLI and dashboard (local compute)"
S=http://localhost:18080
server local 18080 19090 "Compute: local"

expect "version" "$funnel" version
expect "examples list" "$funnel" examples
"$funnel" examples hello-world > hello.json
expect "completion" "$funnel" completion bash

id=$("$funnel" task create -S $S hello.json)
expect "task wait" "$funnel" task wait -S $S "$id"
[ "$(state $S "$id")" = COMPLETE ] || { echo "FAIL hello-world: $(state $S "$id")"; exit 1; }
expect "task get FULL" "$funnel" task get -S $S --view FULL "$id"
expect "task get BASIC" "$funnel" task get -S $S --view BASIC "$id"
expect_fail "task get unknown" "$funnel" task get -S $S does-not-exist

echo "input data" > in.txt
mkdir -p indir && echo a > indir/a.txt && echo b > indir/b.txt
expect "run with inputs and outputs" "$funnel" run -S $S --wait --name cli-io --tag kind=io \
  --in in=$work/in.txt --in-dir dir=$work/indir --out out=$work/out.txt --out-dir odir=$work/outdir \
  --content conf=$work/in.txt -e GREETING=hi --workdir /tmp \
  'cat $in $dir/a.txt $conf > $out && mkdir -p $odir && echo $GREETING > $odir/g.txt'
grep -q "input data" out.txt || { echo "FAIL output file"; exit 1; }
[ "$(cat outdir/g.txt)" = hi ] || { echo "FAIL output directory"; exit 1; }
expect "run with stdin/stdout/stderr" "$funnel" run -S $S --wait --stdin $work/in.txt \
  --stdout $work/stdout.txt --stderr $work/stderr.txt --cpu 1 --ram 0.5 --disk 1 'cat; echo err >&2'
grep -q "input data" stdout.txt || { echo "FAIL stdout file"; exit 1; }
expect "run --exec" "$funnel" run -S $S --wait --exec 'echo exec'
expect "run --print" "$funnel" run --print --name printed 'echo hi'
expect "run --wait-for" "$funnel" run -S $S --wait --wait-for "$id" 'echo after'
expect_fail "run with a failing command" "$funnel" run -S $S --wait 'exit 3'
expect_fail "run with a missing input" "$funnel" run -S $S --wait --in in=$work/missing.txt 'cat $in'

long=$("$funnel" run -S $S 'sleep 300')
expect "task cancel" "$funnel" task cancel -S $S "$long"
for _ in $(seq 1 30); do [ "$(state $S "$long")" = CANCELED ] && break; sleep 1; done
[ "$(state $S "$long")" = CANCELED ] || { echo "FAIL cancel: $(state $S "$long")"; exit 1; }

expect "task list" "$funnel" task list -S $S
expect "task list --all FULL" "$funnel" task list -S $S --all --view FULL
expect "task list filters" "$funnel" task list -S $S --state COMPLETE --name-prefix cli- --tag kind=io --page-size 1
"$funnel" task list -S $S --name-prefix cli- --view BASIC | jq -e '.tasks | length == 1' > /dev/null || { echo "FAIL name prefix"; exit 1; }

# The dashboard reads the server's tasks; drive it through a pseudo terminal
# (expanded task, sort, filter and help menus) and quit with "q".
keys() {
  sleep 3
  for k in j j k $'\r'; do printf '%s' "$k"; sleep 1; done
  sleep 2; printf '\033'; sleep 1
  for k in a H r s; do printf '%s' "$k"; sleep 1; done
  printf '\033'; sleep 1; printf 'f'; sleep 1; printf '\033'; sleep 1
  printf '?'; sleep 1; printf '\033'; sleep 1; printf 'q'; sleep 2
}
if [ "$(uname)" = Darwin ]; then
  keys | TERM=xterm script -q /dev/null "$funnel" dashboard -S $S > dashboard.log 2>&1 || true
else
  keys | TERM=xterm script -q -e -c "$funnel dashboard -S $S" /dev/null > dashboard.log 2>&1 || true
fi
echo "ok   dashboard"

expect "storage put" "$funnel" storage put $work/in.txt file://$work/stored/in.txt
expect "storage get" "$funnel" storage get file://$work/stored/in.txt $work/got.txt
cmp in.txt got.txt
expect "storage stat" "$funnel" storage stat file://$work/stored/in.txt
expect "storage list" "$funnel" storage list file://$work/indir
"$funnel" run --print --in in=$work/in.txt --out out=$work/stored/out.txt 'cat $in > $out' > task.json
expect "storage stat-task" "$funnel" storage stat-task $work/task.json
expect_fail "storage get missing" "$funnel" storage get file://$work/missing.txt $work/x.txt

python3 -m http.server 8765 --bind 127.0.0.1 > http.log 2>&1 &
http=$!
sleep 2
expect "storage get http" "$funnel" storage get http://127.0.0.1:8765/in.txt $work/http.txt
expect "storage stat http" "$funnel" storage stat http://127.0.0.1:8765/in.txt
# funnel run would mount the input at a path with the port's colon, which
# Docker rejects; the task sets the path itself.
cat > http-task.json <<EOF
{"executors": [{"image": "alpine", "command": ["cat", "/data/in.txt"]}],
 "inputs": [{"url": "http://127.0.0.1:8765/in.txt", "path": "/data/in.txt"}]}
EOF
hid=$("$funnel" task create -S $S http-task.json)
expect "task with an http input" "$funnel" task wait -S $S "$hid"
[ "$(state $S "$hid")" = COMPLETE ] || { echo "FAIL http input: $(state $S "$hid")"; exit 1; }

if docker exec funnel-s3 mkdir -p /data/funnel-scenarios 2> /dev/null; then
  # The bucket is a directory of the S3 server (versitygw, posix backend).
  s3=(--config "$src/tests/minio-s3.config.yml")
  expect "storage put s3" "$funnel" storage put "${s3[@]}" $work/in.txt s3://funnel-scenarios/dir/in.txt
  expect "storage list s3" "$funnel" storage list "${s3[@]}" s3://funnel-scenarios/dir/
  expect "storage stat s3" "$funnel" storage stat "${s3[@]}" s3://funnel-scenarios/dir/in.txt
  expect "storage get s3" "$funnel" storage get "${s3[@]}" s3://funnel-scenarios/dir/in.txt $work/s3.txt
  # The same server through the Amazon S3 client
  s3=(--config "$src/tests/amazoncli-minio-s3.config.yml")
  expect "storage put amazon s3" "$funnel" storage put "${s3[@]}" $work/in.txt s3://funnel-scenarios/amazon/in.txt
  expect "storage list amazon s3" "$funnel" storage list "${s3[@]}" s3://funnel-scenarios/amazon/
  expect "storage stat amazon s3" "$funnel" storage stat "${s3[@]}" s3://funnel-scenarios/amazon/in.txt
  expect "storage get amazon s3" "$funnel" storage get "${s3[@]}" s3://funnel-scenarios/amazon/in.txt $work/s3a.txt
  expect_fail "storage get amazon s3 missing" "$funnel" storage get "${s3[@]}" s3://funnel-scenarios/amazon/missing $work/x.txt
else
  echo "skip S3 (no funnel-s3 container: make start-s3)"
fi

kill "$http"

if docker inspect ftp > /dev/null 2>&1; then
  ftp=(--config "$src/tests/ftp.config.yml")
  F=ftp://bob:bob@localhost:8021/scenarios
  expect "storage put ftp" "$funnel" storage put "${ftp[@]}" $work/in.txt $F/in.txt
  expect "storage list ftp" "$funnel" storage list "${ftp[@]}" $F/
  expect "storage stat ftp" "$funnel" storage stat "${ftp[@]}" $F/in.txt
  expect "storage get ftp" "$funnel" storage get "${ftp[@]}" $F/in.txt $work/ftp.txt
  expect_fail "storage get ftp missing" "$funnel" storage get "${ftp[@]}" $F/missing.txt $work/x.txt
else
  echo "skip FTP (no ftp container: make start-ftp)"
fi

echo "== 2. Scheduler mode (manual compute and a node)"
S2=http://localhost:18081
server manual 18081 19091 "Compute: manual" "Scheduler:" "  ScheduleRate: 0.5s"
cat > node.yml <<EOF
RPCClient:
  ServerAddress: localhost:19091
Node:
  ID: scenario-node
  UpdateRate: 1s
  Resources:
    Cpus: 2
    RamGb: 2
    DiskGb: 10
Worker:
  WorkDir: $work/manual/node-work
LocalStorage:
  AllowedDirs: ["$work"]
EOF
"$funnel" node run --config node.yml > node.log 2>&1 &
node=$!
pids+=($node)
expect "run on the node" "$funnel" run -S $S2 --wait 'echo scheduled'
expect "run on the node with an input" "$funnel" run -S $S2 --wait --in in=$work/in.txt 'cat $in'
expect "run too large for the node stays queued" "$funnel" run -S $S2 --cpu 64 'echo never'
stop "$node"

echo "== 3. HPC backends (Slurm, PBS, Grid Engine, HTCondor) with fake commands"
# One script plays all commands of the four schedulers: a job runs its
# script in the background on this machine; HPC_KIND tells PBS's from Grid
# Engine's qsub.
mkdir -p hpc
cat > hpc/fake <<'EOF'
#!/usr/bin/env bash
jobs=$HPC_JOBS
running() { kill -0 "$(cat "$jobs/$1")" 2>/dev/null; }
ids() { ls "$jobs" | grep -v '\.out$' | sort -n; }
# The job IDs a status command asks for: its last argument, comma-separated.
asked() { local a=${*: -1}; echo "${a//,/ }"; }
case $(basename "$0") in
sbatch | qsub | condor_submit)
  id=$(( $(ids | wc -l) + 1 ))
  bash "$1" > "$jobs/$id.out" 2>&1 &
  echo $! > "$jobs/$id"
  case $(basename "$0")/$HPC_KIND in
  sbatch/*) echo "Submitted batch job $id" ;;
  qsub/pbs) echo -n "$id" ;;
  qsub/*) echo "Your job $id (\"funnel\") has been submitted" ;;
  condor_submit/*) printf 'Submitting job(s).\n1 job(s) submitted to cluster %s.\n' "$id" ;;
  esac ;;
scancel | qdel | condor_rm)
  [ -f "$jobs/$1" ] && kill "$(cat "$jobs/$1")" 2>/dev/null
  exit 0 ;;
squeue)
  for id in $(asked "$@"); do running "$id" && echo "$id RUNNING None"; done; exit 0 ;;
sacct)
  for id in $(asked "$@"); do running "$id" || echo "$id COMPLETED"; done; exit 0 ;;
qstat)
  echo "<Data>"
  for id in $(ids); do
    if running "$id"; then st=R; else st=C; fi
    echo "<Job><Job_Id>$id</Job_Id><job_state>$st</job_state><exit_status>0</exit_status></Job>"
  done
  echo "</Data>" ;;
condor_q | condor_history)
  sep=; echo "["
  for id in $(asked "$@"); do
    if running "$id"; then st=2; [ "$(basename "$0")" = condor_q ] || continue
    else st=4; [ "$(basename "$0")" = condor_history ] || continue; fi
    echo "$sep{\"ClusterId\": $id, \"JobStatus\": $st, \"ExitCode\": 0}"; sep=,
  done
  echo "]" ;;
esac
EOF
chmod +x hpc/fake
ln -sf "$funnel" hpc/funnel

# hpc <backend> <config section> <reconcile> <http port> <rpc port> <commands...>
hpc() {
  local kind=$1 section=$2 reconcile=$3 http=$4 rpc=$5
  shift 5
  mkdir -p "hpc/$kind/bin" "hpc/$kind/jobs"
  for c in "$@"; do ln -sf ../../fake "hpc/$kind/bin/$c"; done
  PATH=$work/hpc/$kind/bin:$work/hpc:$PATH HPC_KIND=$kind HPC_JOBS=$work/hpc/$kind/jobs \
    server "$kind" "$http" "$rpc" "Compute: $kind" "$section:" "$reconcile" \
    "  Template: |" "    #!/bin/bash" "    funnel worker run --taskID {{.TaskId}} --config $work/$kind.yml"
  local S=http://localhost:$http
  expect "run on $kind" "$funnel" run -S $S --wait --cpu 1 --ram 1 --disk 1 'echo hpc'
  local id
  id=$("$funnel" run -S $S 'sleep 300')
  for _ in $(seq 1 30); do [ "$(state $S "$id")" = RUNNING ] && break; sleep 1; done
  # Long enough for the reconciler to check the running job.
  sleep 3
  expect "cancel on $kind" "$funnel" task cancel -S $S "$id"
}
# Grid Engine has no reconciler.
hpc slurm Slurm "  ReconcileRate: 1s" 18082 19092 sbatch squeue sacct scancel
hpc pbs PBS "  ReconcileRate: 1s" 18083 19093 qsub qstat qdel
hpc gridengine GridEngine "" 18084 19094 qsub qdel
hpc htcondor HTCondor "  ReconcileRate: 1s" 18085 19095 condor_submit condor_q condor_history condor_rm

echo "== 4. Basic authentication and task access (OwnerOrAdmin)"
server_lines=("  TaskAccess: OwnerOrAdmin" "  BasicAuth:" \
  "    - User: alice" "      Password: alice-pw" "    - User: bob" "      Password: bob-pw" \
  "    - User: admin" "      Password: admin-pw" "      Admin: true")
# The worker reports to the server as the administrator.
client_lines=("  Credential:" "    User: admin" "    Password: admin-pw")
server auth 18086 19096 "Compute: local"
server_lines=()
client_lines=()
A=http://localhost:18086
# want <description> <HTTP status> <curl arguments...>
want() {
  local what=$1 code=$2 got; shift 2
  got=$(curl -s -o "$work/out" -w '%{http_code}' "$@")
  if [ "$got" = "$code" ]; then echo "ok   $what"; else echo "FAIL $what: HTTP $got, want $code"; cat "$work/out"; exit 1; fi
}
task='{"executors": [{"image": "alpine", "command": ["echo", "auth"]}]}'
want "no credentials" 401 $A/v1/tasks
want "wrong password" 401 -u alice:wrong $A/v1/tasks
want "create as alice" 200 -u alice:alice-pw -H 'Content-Type: application/json' -d "$task" $A/v1/tasks
aid=$(jq -r .id "$work/out")
want "get own task" 200 -u alice:alice-pw "$A/v1/tasks/$aid?view=FULL"
want "get another user's task" 403 -u bob:bob-pw "$A/v1/tasks/$aid"
want "cancel another user's task" 403 -u bob:bob-pw -X POST "$A/v1/tasks/$aid:cancel"
want "admin gets any task" 200 -u admin:admin-pw "$A/v1/tasks/$aid"
want "list as bob" 200 -u bob:bob-pw "$A/v1/tasks"
jq -e '(.tasks // []) | length == 0' "$work/out" > /dev/null || { echo "FAIL bob sees alice's task"; exit 1; }
want "unknown task" 404 -u alice:alice-pw "$A/v1/tasks/no-such-task"
want "invalid task" 400 -u alice:alice-pw -H 'Content-Type: application/json' -d '{"executors": []}' $A/v1/tasks
want "malformed JSON" 400 -u alice:alice-pw -H 'Content-Type: application/json' -d '{' $A/v1/tasks
want "health" 200 $A/healthz
want "metrics" 200 $A/metrics
want "dashboard" 200 -H "Accept: text/html" $A/
# /login and /login/token (status codes not checked).
for args in "$A/login" "-u alice:alice-pw $A/login" "$A/login/token" "-X POST $A/login"; do
  curl -s -o /dev/null $args
done
echo "ok   login endpoints"
FUNNEL_SERVER_USER=alice FUNNEL_SERVER_PASSWORD=alice-pw expect "CLI with credentials" "$funnel" task wait -S $A "$aid"
expect_fail "CLI without credentials" "$funnel" task list -S $A
FUNNEL_SERVER_USER=bob FUNNEL_SERVER_PASSWORD=bob-pw expect_fail "CLI as another user" "$funnel" task get -S $A "$aid"

echo "== 5. Standalone worker (task file, base64 task, scratch directory)"
mkdir -p standalone
cat > standalone/task.json <<EOF
{"id": "standalone-1",
 "executors": [{"image": "alpine", "command": ["sh", "-c", "mkdir -p /data/out && cat /data/in.txt > /data/out/out.txt"]}],
 "inputs": [{"url": "$work/in.txt", "path": "/data/in.txt"}],
 "outputs": [{"url": "$work/standalone/result", "path": "/data/out", "type": "DIRECTORY"}],
 "volumes": ["/scratch-volume"]}
EOF
cat > standalone/worker.yml <<EOF
EventWriters: [log]
Worker:
  WorkDir: $work/standalone/work
  ScratchPath: $work/standalone/scratch
LocalStorage:
  AllowedDirs: ["$work"]
EOF
expect "worker with a task file" "$funnel" worker run --config standalone/worker.yml --taskFile standalone/task.json
cmp in.txt standalone/result/out.txt
rm standalone/result/out.txt
expect "worker with a base64 task" "$funnel" worker run --config standalone/worker.yml \
  --taskBase64 "$(base64 < standalone/task.json | tr -d '\n')"
cmp in.txt standalone/result/out.txt
expect_fail "worker with an unknown image" "$funnel" worker run --config standalone/worker.yml \
  --taskBase64 "$(echo '{"id": "standalone-2", "executors": [{"image": "funnel-scenarios/no-such-image:0", "command": ["true"]}]}' | base64 | tr -d '\n')"

echo "== done"
