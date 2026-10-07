---
title: Tasks
menu:
  main:
    identifier: tasks
    weight: -70
---

# Tasks

A task defines a unit of work:

- metadata
- input files to download
- a sequence of Docker containers + commands to run,
- output files to upload
- state
- logs

The example task below downloads a file named `hello.txt` from S3 and calls `cat hello.txt` using the [alpine][alpine] container. This task also writes the executor's stdout to a file, and uploads the stdout to s3.
The `#` comments only explain the fields; remove them before sending the task.

```
{
  "name": "Hello world",
  "inputs": [{
    # URL to download file from.
    "url": "s3://funnel-bucket/hello.txt",
    # Path to download file to.
    "path": "/inputs/hello.txt"
  }],
  "outputs": [{
    # URL to upload file to.
    "url": "s3://funnel-bucket/output.txt",
    # Local path to upload file from.
    "path": "/outputs/stdout"
  }],
  "executors": [{
      # Container image name.
      "image": "alpine",
      # Command to run (argv).
      "command": ["cat", "/inputs/hello.txt"],
      # Capture the stdout of the command to /outputs/stdout
      "stdout": "/outputs/stdout"
  }]
}
```

Tasks have multiple "executors"; containers and commands run in a sequence. 
Funnel runs executors via Docker (or as Kubernetes Jobs with the `kubernetes`
compute backend). With the default Docker command (`Worker.Container.RunCommand`)
an executor's root filesystem is read-only: it can write only to the paths of
its inputs, outputs and volumes and to `/tmp`, which is private to each executor (unless the task declares a
volume at `/tmp`, which is then shared by its executors).

Tasks also have state and logs (the responses below are shortened; the server
returns every field of the task, using the `snake_case` field names of the
TES specification, and empty fields as `""`, `[]`, `{}` or `null`):
```
{
  "id": "b85khc2rl6qkqbhg8vig",
  "state": "COMPLETE",
  "name": "Hello world",
  "inputs": [
    {
      "url": "s3://funnel-bucket/hello.txt",
      "path": "/inputs/hello.txt"
    }
  ],
  "outputs": [
    {
      "url": "s3://funnel-bucket/output.txt",
      "path": "/outputs/stdout"
    }
  ],
  "executors": [
    {
      "image": "alpine",
      "command": [
        "cat",
        "/inputs/hello.txt"
      ],
      "stdout": "/outputs/stdout"
    }
  ],
  "logs": [
    {
      "logs": [
        {
          "start_time": "2017-11-14T11:49:05.127885125-08:00",
          "end_time": "2017-11-14T11:49:08.484461502-08:00",
          "exit_code": 0,
          "stdout": "Hello, Funnel!\n"
        }
      ],
      "start_time": "2017-11-14T11:49:04.433593468-08:00",
      "end_time": "2017-11-14T11:49:08.487707039-08:00"
    }
  ],
  "creation_time": "2017-11-14T11:49:04.427163701-08:00"
}
```

There are logs for each task attempt and each executor. Notice that the stdout is
conveniently captured by `logs[0].logs[0].stdout`.

The `funnel task get` and `funnel task list` commands print the same data, but
with `camelCase` field names (`creationTime`, `startTime`, …) and without empty
fields.

### Task API

The API lets you create, get, list, and cancel tasks.

### Create
```
POST /v1/tasks
{
  "name": "Hello world",
  "inputs": [{
    "url": "s3://funnel-bucket/hello.txt",
    "path": "/inputs/hello.txt"
  }],
  "outputs": [{
    "url": "s3://funnel-bucket/output.txt",
    "path": "/outputs/stdout"
  }],
  "executors": [{
      "image": "alpine",
      "command": ["cat", "/inputs/hello.txt"],
      "stdout": "/outputs/stdout"
  }]
}


# The response contains the task ID:
{"id": "b85khc2rl6qkqbhg8vig"}
```

### Get
```
GET /v1/tasks/b85khc2rl6qkqbhg8vig

{"id": "b85khc2rl6qkqbhg8vig", "state": "COMPLETE", "name": "", "executors": [], ...}
```

By default, the minimal task view is returned: only the ID and state are
filled in, all other fields are empty.
In order to get the original task with some basic logs, use the "BASIC" task view:
```
GET /v1/tasks/b85khc2rl6qkqbhg8vig?view=BASIC
{
  "id": "b85khc2rl6qkqbhg8vig",
  "state": "COMPLETE",
  "name": "Hello world",
  "inputs": [
    {
      "url": "s3://funnel-bucket/hello.txt",
      "path": "/inputs/hello.txt"
    }
  ],
  "outputs": [
    {
      "url": "s3://funnel-bucket/output.txt",
      "path": "/outputs/stdout"
    }
  ],
  "executors": [
    {
      "image": "alpine",
      "command": [
        "cat",
        "/inputs/hello.txt"
      ],
      "stdout": "/outputs/stdout"
    }
  ],
  "logs": [
    {
      "logs": [
        {
          "start_time": "2017-11-14T11:49:05.127885125-08:00",
          "end_time": "2017-11-14T11:49:08.484461502-08:00",
          "exit_code": 0,
          "stdout": ""
        }
      ],
      "start_time": "2017-11-14T11:49:04.433593468-08:00",
      "end_time": "2017-11-14T11:49:08.487707039-08:00",
      "system_logs": []
    }
  ],
  "creation_time": "2017-11-14T11:49:04.427163701-08:00"
}
```

The "BASIC" view leaves out the fields that may be large: executor stdout/stderr,
system logs and the `content` of inputs.
In order to get everything, use the "FULL" view:
```
GET /v1/tasks/b85khc2rl6qkqbhg8vig?view=FULL
{
  "id": "b85khc2rl6qkqbhg8vig",
  "state": "COMPLETE",
  "name": "Hello world",
  "inputs": [
    {
      "url": "s3://funnel-bucket/hello.txt",
      "path": "/inputs/hello.txt"
    }
  ],
  "outputs": [
    {
      "url": "s3://funnel-bucket/output.txt",
      "path": "/outputs/stdout"
    }
  ],
  "executors": [
    {
      "image": "alpine",
      "command": [
        "cat",
        "/inputs/hello.txt"
      ],
      "stdout": "/outputs/stdout"
    }
  ],
  "logs": [
    {
      "logs": [
        {
          "start_time": "2017-11-14T11:49:05.127885125-08:00",
          "end_time": "2017-11-14T11:49:08.484461502-08:00",
          "exit_code": 0,
          "stdout": "Hello, Funnel!\n"
        }
      ],
      "start_time": "2017-11-14T11:49:04.433593468-08:00",
      "end_time": "2017-11-14T11:49:08.487707039-08:00",
      "system_logs": [
        "level='info' msg='Version' ...",
        "level='info' msg='Running command' ..."
      ]
    }
  ],
  "creation_time": "2017-11-14T11:49:04.427163701-08:00"
}
```

### List
```
GET /v1/tasks
{
  "tasks": [
    {
      "id": "b85l8tirl6qkqbhg8vj0",
      "state": "COMPLETE"
    },
    {
      "id": "b85khc2rl6qkqbhg8vig",
      "state": "COMPLETE"
    },
    {
      "id": "b85kgt2rl6qkpuptua70",
      "state": "SYSTEM_ERROR"
    },
    {
      "id": "b857gnirl6qjfou61fh0",
      "state": "SYSTEM_ERROR"
    }
  ]
}
```

List has the same task views as Get: MINIMAL, BASIC, and FULL
(`GET /v1/tasks?view=BASIC`).

The task list is paginated (`page_size`, default 256; the next page is
requested with `page_token`):
```
GET /v1/tasks?page_token=1h123h12j2h3k
{
  "next_page_token": "1n3n1j23k12n3k123",
  "tasks": [
    {
      "id": "b85l8tirl6qkqbhg8vj0",
      "state": "COMPLETE"
    },
    # ... more tasks here ...
  ]
}
```

### Cancel 

Tasks cannot be modified by the user after creation, with one exception – they can be canceled.
```
POST /v1/tasks/b85l8tirl6qkqbhg8vj0:cancel
```


### Full task spec

Here's a more detailed description of a task.  
For a full, in-depth spec, read the TES standard's [OpenAPI specification](https://github.com/ga4gh/task-execution-schemas/blob/develop/openapi/task_execution_service.openapi.yaml).

```
{
    # The task's ID. Set by the server.
    # Output only.
    "id": "1234567",

    # The task's state. Possible states:
    #   UNKNOWN
    #   QUEUED
    #   INITIALIZING
    #   RUNNING
    #   PAUSED
    #   COMPLETE
    #   EXECUTOR_ERROR
    #   SYSTEM_ERROR
    #   CANCELED
    #   PREEMPTED
    #   CANCELING
    #
    # Output only.
    "state": "QUEUED",

    # Metadata
    "name":        "Task name.",
    "description": "Task description.",
    "tags": {
      "custom-tag-1": "tag-value-1",
      "custom-tag-2": "tag-value-2"
    },

    # Resource requests
    "resources": {
      # Number of CPU cores requested.
      "cpu_cores": 1,

      # RAM request, in gigabytes.
      "ram_gb":    1.0,

      # Disk space request, in gigabytes.
      "disk_gb":   100.0,

      # Request preemptible machines,
      # e.g. preemptible VM in Google Cloud, an instance from the AWS Spot Market, etc.
      "preemptible": false,

      # Request that the task run in these compute zones.
      "zones": ["zone1", "zone2"]
    },

    # Input files will be downloaded by the worker.
    # This example uses s3, but Funnel supports multiple filesystems.
    "inputs": [
      {
        "name": "Input file.",
        "description": "Input file description.",

        # URL to download file from.
        "url":  "s3://my-bucket/object/path/file.txt",
        # Path to download file to.
        "path": "/container/input.txt"
      },
      {
        "name": "Input directory.",
        "description": "Directories are also supported.",
        "url":  "s3://my-bucket/my-data/",
        "path": "/inputs/my-data/",
        "type": "DIRECTORY"
      },

      # A task may include the file content directly in the task message.
      # This is sometimes useful for small files such as scripts,
      # which you want to include without talking directly to the filesystem.
      {
        "path": "/inputs/script.py",
        "content": "import socket; print(socket.gethostname())"
      }
    ],

    # Output files will be uploaded to storage by the worker.
    "outputs": [
      {
        "name": "Output file.",
        "description": "Output file description.",
        "url":  "s3://my-bucket/output-data/results.txt",
        "path": "/outputs/results.txt"
      },
      {
        "name": "Output directory.",
        "description": "Directories are also supported.",
        "url":  "s3://my-bucket/output-data/output-dir/",
        "path": "/outputs/data-dir/",
        "type": "DIRECTORY"
      }
    ],

    # Executors define a sequence of containers + commands to run.
    # Execution stops on the first non-zero exit code
    # (unless the executor sets "ignore_error": true).
    "executors": [
      {
        # Container image name.
        # Funnel supports running executor containers via Docker.
        "image": "ubuntu",

        # Command arguments (argv).
        # The first item is the executable to run.
        "command": ["my-tool-1", "/container/input"],

        # Local file path to read stdin from.
        "stdin": "/inputs/stdin.txt",

        # Local file path to write stdout to.
        "stdout": "/container/output",

        # Local file path to write stderr to.
        "stderr": "/container/stderr",

        # Set the working directory before executing the command.
        "workdir": "/data/workdir",

        # Environment variables
        "env": {
          "ENV1": "value1",
          "ENV2": "value2"
        }
      },

      # Second executor runs after the first completes, on the same machine.
      {
        "image": "ubuntu",
        "command": ["cat", "/container/input"],
        "stdout": "/container/output",
        "stderr": "/container/stderr",
        "workdir": "/tmp"
      }
    ],

    # Date/time the task was created.
    # Set by the server.
    # Output only.
    "creation_time": "2017-11-14T11:49:04.427163701-08:00",

    # Task logs.
    # Output only.
    #
    # If there's a system error, the task may be attempted multiple times,
    # so this field is a list of attempts. In most cases, there will be only
    # one or zero entries here.
    "logs": [
      {
        # Attempt start/end times, in RFC3339 format.
        "start_time": "2017-11-14T11:49:04.433593468-08:00",
        "end_time": "2017-11-14T11:49:08.487707039-08:00",

        # Arbitrary metadata set by Funnel.
        "metadata": {
          "hostname": "worker-1"
        },

        # Arbitrary system logs which Funnel thinks are useful to the user.
        # Only available in the FULL task view.
        "system_logs": [
          "level='info' msg='Running command' ... cmd='docker run ... alpine cmd arg1 arg2'"
        ],

        # Log of files uploaded to storage by the worker,
        # including all files in directories, with file sizes.
        "outputs": [
          {
            "url": "s3://my-bucket/output-data/results.txt",
            "path": "/outputs/results.txt",
            "size_bytes": "123"
          },
          {
            "url": "s3://my-bucket/output-data/output-dir/file1.txt",
            "path": "/outputs/data-dir/file1.txt",
            "size_bytes": "123"
          },
          {
            "url": "s3://my-bucket/output-data/output-dir/subdir/file3.txt",
            "path": "/outputs/data-dir/subdir/file3.txt",
            "size_bytes": "123"
          }
        ],

        # Executor logs. One entry per executor.
        "logs": [
          {
            # Executor start/end time, in RFC3339 format.
            "start_time": "2017-11-14T11:49:05.127885125-08:00",
            "end_time": "2017-11-14T11:49:08.484461502-08:00",

            # Executor stdout/err. Only available in the FULL task view.
            #
            # There is a size limit for these fields, which is configurable
            # (Worker.LogTailSize) and defaults to 10 KB. If more than 10 KB
            # is generated, only the tail will be logged. If the full output
            # is needed, the task may use Executor.stdout and an output to
            # upload the full content to storage.
            "stdout": "Hello, Funnel!",
            "stderr": "",

            # Exit code
            "exit_code": 0
          },
          {
            "start_time": "2017-11-14T11:49:05.127885125-08:00",
            "end_time": "2017-11-14T11:49:08.484461502-08:00",
            "stdout": "Hello, Funnel!\n",
            "exit_code": 0
          }
        ]
      }
    ]
}
```

[alpine]: https://hub.docker.com/_/alpine/
