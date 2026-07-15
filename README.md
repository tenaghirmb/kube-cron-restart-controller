<!-- markdownlint-disable-next-line MD033 -->
<h1>kube-cron-restart-controller</h1>

[![TOC Automation](https://github.com/tenaghirmb/kube-cron-restart-controller/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/tenaghirmb/kube-cron-restart-controller/actions/workflows/main.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](https://opensource.org)
[![BuyMeACoffee](https://raw.githubusercontent.com/pachadotdev/buymeacoffee-badges/main/bmc-donate-yellow.svg)](https://www.buymeacoffee.com/tenaghirmb)

**Advanced Cron Restart Controller** is an industrial-grade Kubernetes Operator designed to orchestrate scheduled workload maintenance at scale. Built with **Go** and **Kubebuilder**, it replaces the resource-heavy native CronJob pattern with a high-concurrency **In-Memory Scheduling Engine** coupled with **Atomic Status-Gatekeeping** to ensure 100% execution integrity in distributed environments.

# Table of Contents
<!--ts-->
   * [Features](#features)
   * [Architecture Overview](#architecture-overview)
      * [Why Not Native CronJobs?](#why-not-native-cronjobs)
      * [Operator Sequence Diagram](#operator-sequence-diagram)
   * [Tech Stack](#tech-stack)
   * [Prerequisites](#prerequisites)
   * [Installation](#installation)
      * [Using Helm](#using-helm)
      * [Using Kustomize](#using-kustomize)
   * [Usage](#usage)
   * [Configuration](#configuration)
      * [timezone](#timezone)
      * [restartTargetRef](#restarttargetref)
      * [excludeDates](#excludedates)
      * [schedule](#schedule)
      * [misfirePolicy](#misfirepolicy)
      * [misfireDeadWindowMinutes](#misfiredeadwindowminutes)
      * [cron expression](#cron-expression)
         * [Special Characters](#special-characters)
         * [Predefined Schedules](#predefined-schedules)
         * [Intervals](#intervals)
   * [Contributing](#contributing)
   * [Licensing](#licensing)
<!--te-->

## Features

- **Scheduled Restarts**: Automatically restart resources on a predefined schedule.
- **Resource Support**: Supports Deployments, StatefulSets and DaemonSets which can be restarted using the `kubectl rollout restart` command.
- **Custom Schedules**: Define custom restart schedules using Cron syntax.
- **Flexible Time Configuration**: Supports skipping specified dates.

## Architecture Overview

A dual-layer orchestration engine combining a High-Performance in-Memory Registry with Kubernetes Atomic Status-Gatekeeping.

### Why Not Native CronJobs?

- **Resource Footprint**: Consumes 90% less etcd storage compared to 10000+ native CronJob objects.
- **Execution Latency**: Sub-millisecond dispatching vs native kube-controller-manager polling delays.
- **Reliability & Edge Cases Split-Brain Protection**: Leveraging Conditional Patching with ResourceVersion (OCC, Optimistic Concurrency Control) to ensure strict mutual exclusion in HA deployments.
- **Misfire Compensation**: Automatically detects historical schedule gaps upon cold-start, ensuring critical maintenance windows are never missed.

### Operator Sequence Diagram

```text
[Manager.Start]
       │
       ├─► [Async Start Cache] ────► Establish Watch, sync K8s resources
       │
       └─► [Async Start CronManager.Start]
                 │
                 ├─► [Action A: Immediately Start Engine] 
                 │      │
                 │      └─► cm.cronExecutor.Run() ──► Start Regular Clock (e.g., Locks 13:10 Tick)
                 │
                 └─► [Action B: Async Misfire Compensation Loop]
                        │
                        ▼
                 [cm.misfireCompensate()]
                        │
                        ├─► 1. Fetch Snapshot List from Client Cache
                        │
                        ├─► 2. Queue in Concurrency Slot (Semaphore)
                        │
                        └─► 3. Execute Async Worker (e.g., Runs at 13:10:00)
                                    │
                                    ▼
                       [Real-time Check Barrier]
                                    │
                        ┌───────────┴───────────┐
                        ▼                       ▼
            [13:10:00 Regular Clock]    [13:10:00 Async Compensate Worker]
                        │                       │
                        │                       ├─► cm.client.Get() (Fetch Latest State)
                        │                       │
                        ▼                       ▼
                [Try Status Patch]      [Try Status Patch]
             (ResourceVersion: 1001) (ResourceVersion: 1001)
                        │                       │
                        ▼                       ▼
             ┌─────────────────────┐ ┌─────────────────────┐
             │ Win the Race        │ │ Lose the Race       │
             │ Status Patched!     │ │ K8s API returns:    │
             │ RV updates to 1002  │ │ "Object Modified"   │
             └─────────────────────┘ └─────────────────────┘
                                                        │
                                                        ▼
                                             [OCC Conflict Defense]
                                             Abort Compensation Safely!
```

## Tech Stack

| Component | Technology | Purpose |
| :-- | :-- | :-- |
| Language | Golang 1.26 | Core runtime environment |
| Framework | Kubebuilder v4 | Operator scaffolding and boilerplate generation |
| Controller Runtime | controller-runtime | Reconciliation pattern implementation |
| Cron Library | robfig/cron | Enhanced cron expression parsing and scheduling |
| Kubernetes API | client-go | Kubernetes resource manipulation |
| Dependency Management | Go Modules | Package version management |
| Build System | Make | Compilation and artifact generation |
| Container Runtime | Docker | Operator containerization |
| Package Manager | Helm 4 | Cluster installation and upgrades |

## Prerequisites

| Tool | Minimum Version | Purpose |
| :-- | :-- | :-- |
| Kubernetes | v1.34+ | Kubernetes cluster for operator to run on |
| Docker | 23.0+ | Building container images (if deploying from source) |
| Go | v1.24.0+ | Building from source |
| Helm | v3.x+ | Installing via Helm chart |

## Installation

### Using Helm

```bash
# 1. From Source
make helm-deploy IMG=docker.io/tenaghirmb/cronrestart:v2.0.0
# 2. Using Repo
helm repo add helm-charts https://tenaghirmb.github.io/cronrestart/
helm repo update
helm install cronrestart helm-charts/cronrestart
```

### Using Kustomize

``` bash
make deploy-test
make deploy-prod
```

## Usage

Try out the examples in the `config/samples` folder.

1. Deploy Samples

    ```bash
    # using Kustomize
    kubectl apply -k config/samples
    # Or using Makefile
    make deploy-samples
    ```

2. Check the status of the deployment

    ```bash
    [0] % kubectl describe deploy nginx-deployment-basic
    Name:                   nginx-deployment-basic
    Namespace:              default
    CreationTimestamp:      Sat, 11 Jul 2026 15:52:41 +0800
    Labels:                 app=nginx
    Annotations:            deployment.kubernetes.io/revision: 1
    Selector:               app=nginx
    Replicas:               1 desired | 1 updated | 1 total | 1 available | 0 unavailable
    StrategyType:           RollingUpdate
    MinReadySeconds:        0
    RollingUpdateStrategy:  25% max unavailable, 25% max surge
    Pod Template:
      Labels:  app=nginx
      Containers:
      nginx:
        Image:      nginx:alpine
        Port:       80/TCP
        Host Port:  0/TCP
        Limits:
          cpu:     500m
          memory:  128Mi
        Requests:
          cpu:         250m
          memory:      64Mi
        Environment:   <none>
        Mounts:        <none>
      Volumes:         <none>
      Node-Selectors:  <none>
      Tolerations:     <none>
    Conditions:
      Type           Status  Reason
      ----           ------  ------
      Available      True    MinimumReplicasAvailable
      Progressing    True    NewReplicaSetAvailable
    OldReplicaSets:  <none>
    NewReplicaSet:   nginx-deployment-basic-78d967d4dc (1/1 replicas created)
    Events:
      Type    Reason             Age   From                   Message
      ----    ------             ----  ----                   -------
      Normal  ScalingReplicaSet  29s   deployment-controller  Scaled up replica set nginx-deployment-basic-78d967d4dc from 0 to 1
    ```

3. Check the restart event

    ```bash
    [0] % kubectl describe deploy nginx-deployment-basic
    Name:                   nginx-deployment-basic
    Namespace:              default
    CreationTimestamp:      Sat, 11 Jul 2026 15:52:41 +0800
    Labels:                 app=nginx
    Annotations:            deployment.kubernetes.io/revision: 4
    Selector:               app=nginx
    Replicas:               1 desired | 1 updated | 1 total | 1 available | 0 unavailable
    StrategyType:           RollingUpdate
    MinReadySeconds:        0
    RollingUpdateStrategy:  25% max unavailable, 25% max surge
    Pod Template:
      Labels:       app=nginx
      Annotations:  kubectl.kubernetes.io/restartedAt: 2026-07-11T09:11:39Z
      Containers:
      nginx:
        Image:      nginx:alpine
        Port:       80/TCP
        Host Port:  0/TCP
        Limits:
          cpu:     500m
          memory:  128Mi
        Requests:
          cpu:         250m
          memory:      64Mi
        Environment:   <none>
        Mounts:        <none>
      Volumes:         <none>
      Node-Selectors:  <none>
      Tolerations:     <none>
    Conditions:
      Type           Status  Reason
      ----           ------  ------
      Available      True    MinimumReplicasAvailable
      Progressing    True    NewReplicaSetAvailable
    OldReplicaSets:  nginx-deployment-basic-78d967d4dc (0/0 replicas created), nginx-deployment-basic-7fd79bd599 (0/0 replicas created), nginx-deployment-basic-797ddf8cfb (0/0 replicas created)
    NewReplicaSet:   nginx-deployment-basic-59fd945788 (1/1 replicas created)
    Events:
      Type    Reason             Age   From                   Message
      ----    ------             ----  ----                   -------
      Normal  ScalingReplicaSet  46s   deployment-controller  Scaled up replica set nginx-deployment-basic-59fd945788 from 0 to 1
      Normal  ScalingReplicaSet  45s   deployment-controller  Scaled down replica set nginx-deployment-basic-797ddf8cfb from 1 to 0
    ```

4. Describe the cronrestarter

    ```bash
    [0] % kubectl describe cronrestarters cronrestarter-sample
    Name:         cronrestarter-sample
    Namespace:    default
    Labels:       app.kubernetes.io/managed-by=kustomize
                  app.kubernetes.io/name=cron-restart
    Annotations:  <none>
    API Version:  autorestart.uni.com/v1
    Kind:         CronRestarter
    Metadata:
      Creation Timestamp:  2026-07-11T07:52:42Z
      Finalizers:
        cronrestarter.finalizers.uni.com
      Generation:        1
      Resource Version:  3525084
      UID:               6f67a902-69c5-4553-9099-823b0ea55a0b
    Spec:
      Exclude Dates:
        * * 4 6 *
        * * * * 5
      Misfire Dead Window Minutes:  3
      Misfire Policy:               FireAndProceed
      Restart Target Ref:
        API Version:  apps/v1
        Kind:         Deployment
        Name:         nginx-deployment-basic
      Schedule:       */10 15-23 * * *
      Timezone:       Asia/Shanghai
    Status:
      Conditions:
        Last Probe Time:    2026-07-11T08:00:01Z
        Message:            cron restarter job cronrestarter-sample executed successfully.
        State:              Succeed
        Last Probe Time:    2026-07-11T08:10:03Z
        Message:            cron restarter job cronrestarter-sample executed successfully.
        State:              Succeed
        Last Probe Time:    2026-07-11T09:11:39Z
        Message:            cron restarter job cronrestarter-sample executed successfully.
        State:              Succeed
      Entry Id:             0
      Last Execution Time:  2026-07-11T09:11:39Z
      Last Tick Timestamp:  2026-07-11T09:11:38Z
      Message:              cron restarter job cronrestarter-sample executed successfully.
      State:                Succeed
    Events:
      Type    Reason   Age   From           Message
      ----    ------   ----  ----           -------
      Normal  Succeed  87s   CronRestarter  cron restarter job cronrestarter-sample executed successfully.
    ```

    ```bash
    [0] % kubectl describe cronrestarters cronrestarter-sample
    Name:         cronrestarter-sample
    Namespace:    default
    Labels:       app.kubernetes.io/managed-by=kustomize
                  app.kubernetes.io/name=cron-restart
    Annotations:  <none>
    API Version:  autorestart.uni.com/v1
    Kind:         CronRestarter
    Metadata:
      Creation Timestamp:  2026-07-11T07:52:42Z
      Finalizers:
        cronrestarter.finalizers.uni.com
      Generation:        1
      Resource Version:  3525271
      UID:               6f67a902-69c5-4553-9099-823b0ea55a0b
    Spec:
      Exclude Dates:
        * * 4 6 *
        * * * * 5
      Misfire Dead Window Minutes:  3
      Misfire Policy:               FireAndProceed
      Restart Target Ref:
        API Version:  apps/v1
        Kind:         Deployment
        Name:         nginx-deployment-basic
      Schedule:       */10 15-23 * * *
      Timezone:       Asia/Shanghai
    Status:
      Conditions:
        Last Probe Time:    2026-07-11T08:00:01Z
        Message:            cron restarter job cronrestarter-sample executed successfully.
        State:              Succeed
        Last Probe Time:    2026-07-11T08:10:03Z
        Message:            cron restarter job cronrestarter-sample executed successfully.
        State:              Succeed
        Last Probe Time:    2026-07-11T09:11:39Z
        Message:            cron restarter job cronrestarter-sample executed successfully.
        State:              Succeed
        Last Probe Time:    2026-07-11T09:20:01Z
        Message:            cron restarter job cronrestarter-sample executed successfully.
        State:              Succeed
      Entry Id:             1
      Last Execution Time:  2026-07-11T09:20:01Z
      Last Tick Timestamp:  2026-07-11T09:20:00Z
      Message:              cron restarter job cronrestarter-sample executed successfully.
      State:                Succeed
    Events:
      Type    Reason   Age                  From           Message
      ----    ------   ----                 ----           -------
      Normal  Succeed  14s (x2 over 8m36s)  CronRestarter  cron restarter job cronrestarter-sample executed successfully.
    ```

    > The `Status.Conditions` holds the latest 10 restarting execution results;
    > The `Status.State` indicates its execution status. When the `State` is `Succeed`, it means the last execution was successful. When the `State` is `Submitted`, it means the cronrestart job has been submitted to the cron engine and is waiting to be executed. When the `State` is `Failed`, it means the last execution failed;
    > The `Status.EntryId` indicates the sequential number of the cronrestart job in the cron engine. When the `EntryId` is `0`, it means the job was compensated by CronManager.

5. Uninstall Samples

    ```bash
    make undeploy-samples
    ```

6. Check controller's log

    ```log
    2026-07-11T17:11:36.445+0800	[INFO][PID:4325]	cmd/main.go:185	starting manager
    2026-07-11T17:11:36.446+0800	[INFO][PID:4325]	manager/server.go:83	starting server	{"name": "health probe", "addr": "[::]:8081"}
    2026-07-11T17:11:36.446+0800	[INFO][PID:4325]	controller-runtime.metrics	server/server.go:208	Starting metrics server
    2026-07-11T17:11:36.447+0800	[INFO][PID:4325]	manager/server.go:83	starting server	{"name": "pprof", "addr": "127.0.0.1:7007"}
    2026-07-11T17:11:36.447+0800	[INFO][PID:4325]	controller/cronmanager.go:57	Starting CronManager component...
    2026-07-11T17:11:36.447+0800	[INFO][PID:4325]	controller/cronmanager.go:61	Regular Cron Engine Clock has been activated successfully.
    2026-07-11T17:11:36.447+0800	[INFO][PID:4325]	controller-runtime.metrics	server/server.go:247	Serving metrics server	{"bindAddress": ":8443", "secure": false}
    2026-07-11T17:11:36.447+0800	[INFO][PID:4325]	controller/cronmanager.go:147	Starting asynchronous misfire compensation loop...
    2026-07-11T17:11:36.448+0800	[INFO][PID:4325]	controller/cronmanager.go:79	GC loop initialized to run every 10m0s
    2026-07-11T17:11:36.448+0800	[INFO][PID:4325]	controller/controller.go:369	Starting EventSource	{"controller": "cronrestarter", "controllerGroup": "autorestart.uni.com", "controllerKind": "CronRestarter", "source": "kind source: *v1.CronRestarter"}
    2026-07-11T17:11:37.943+0800	[INFO][PID:4325]	controller/cronmanager.go:203	[Compensate Worker] Confirmed missed schedule for default/cronrestarter-sample. Executing compensatory run.
    2026-07-11T17:11:38.043+0800	[INFO][PID:4325]	controller/controller.go:302	Starting Controller	{"controller": "cronrestarter", "controllerGroup": "autorestart.uni.com", "controllerKind": "CronRestarter"}
    2026-07-11T17:11:38.043+0800	[INFO][PID:4325]	controller/controller.go:305	Starting workers	{"controller": "cronrestarter", "controllerGroup": "autorestart.uni.com", "controllerKind": "CronRestarter", "worker count": 1}
    2026-07-11T17:11:38.044+0800	[INFO][PID:4325]	controller/cronrestarter_controller.go:83	Start to handle cronRestarter cronrestarter-sample in default namespace
    2026-07-11T17:11:38.045+0800	[INFO][PID:4325]	controller/cronmanager.go:40	cronRestarter job cronrestarter-sample of cronRestarter cronrestarter-sample in default updated, 1 active jobs exist
    2026-07-11T17:11:39.572+0800	[INFO][PID:4325]	controller/cronjob.go:240	Deployment nginx-deployment-basic in namespace default has been restarted successfully. cronRestarter: cronrestarter-sample id: 
    2026-07-11T17:11:39.980+0800	[INFO][PID:4325]	controller/cronmanager.go:182	All asynchronous misfire compensations completed.
    2026-07-11T17:11:39.979+0800	[DEBUG][PID:4325]	events	recorder/recorder.go:116	cron restarter job cronrestarter-sample executed successfully.	{"type": "Normal", "object": {"kind":"CronRestarter","namespace":"default","name":"cronrestarter-sample","uid":"6f67a902-69c5-4553-9099-823b0ea55a0b","apiVersion":"autorestart.uni.com/v1","resourceVersion":"3525084"}, "reason": "Succeed"}
    2026-07-11T17:20:01.028+0800	[INFO][PID:4325]	controller/cronjob.go:240	Deployment nginx-deployment-basic in namespace default has been restarted successfully. cronRestarter: cronrestarter-sample id: 2cd097d5-51a3-54b2-85e7-7d7553c103f2
    2026-07-11T17:20:01.841+0800	[DEBUG][PID:4325]	events	recorder/recorder.go:116	cron restarter job cronrestarter-sample executed successfully.	{"type": "Normal", "object": {"kind":"CronRestarter","namespace":"default","name":"cronrestarter-sample","uid":"6f67a902-69c5-4553-9099-823b0ea55a0b","apiVersion":"autorestart.uni.com/v1","resourceVersion":"3525271"}, "reason": "Succeed"}
    2026-07-11T17:21:36.461+0800	[INFO][PID:4325]	controller/cronmanager.go:84	Triggering routine garbage collection...
    2026-07-11T17:23:27.525+0800	[INFO][PID:4325]	controller/cronrestarter_controller.go:83	Start to handle cronRestarter cronrestarter-sample in default namespace
    2026-07-11T17:23:27.526+0800	[INFO][PID:4325]	controller/cronrestarter_controller.go:96	cronRestarter cronrestarter-sample in default namespace is marked to be deleted
    2026-07-11T17:23:27.526+0800	[INFO][PID:4325]	controller/cronmanager.go:50	Remove cronRestarter job cronrestarter-sample of cronRestarter cronrestarter-sample in default from jobQueue,0 active jobs left
    2026-07-11T17:23:27.734+0800	[INFO][PID:4325]	controller/cronrestarter_controller.go:109	Remove finalizer for cronRestarter cronrestarter-sample in default namespace
    2026-07-11T17:23:27.734+0800	[INFO][PID:4325]	controller/cronrestarter_controller.go:111	cronRestarter cronrestarter-sample in default namespace has been finalized successfully. cronId: 2cd097d5-51a3-54b2-85e7-7d7553c103f2
    2026-07-11T17:23:27.735+0800	[INFO][PID:4325]	controller/cronrestarter_controller.go:83	Start to handle cronRestarter cronrestarter-sample in default namespace
    2026-07-11T17:31:36.476+0800	[INFO][PID:4325]	controller/cronmanager.go:84	Triggering routine garbage collection...
    ```

## Configuration

The following example demonstrates how to configure a `CronRestarter`.

```bash
apiVersion: autorestart.uni.com/v1
kind: CronRestarter
metadata:
  labels:
    app.kubernetes.io/name: cron-restart
    app.kubernetes.io/managed-by: kustomize
  name: cronrestart-sample
spec:
  timezone: Asia/Shanghai
  restartTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: nginx-deployment-basic
  excludeDates:
   # exclude June 4th
   - "* * 4 6 *"
   # exclude every Friday 
   - "* * * * 5"
  schedule: "*/10 15-23 * * *"
  misfirePolicy: FireAndProceed
  misfireDeadWindowMinutes: 3
```

### timezone

The `timezone` field specifies the time zone of the task. If schedule contains '@', the timezone will be ignored. Otherwise, the timezone will be used to determine the schedule.

### restartTargetRef

The `restartTargetRef` field specifies the workload to restart. If the workload supports the `kubectl rollout restart` command (such as `Deployment` and `StatefulSet`), `CronRestarter` should work well.

### excludeDates

The `excludeDates` field is an array of dates. The job will skip execution when the date matches. The minimum unit is a day. If you want to skip a specific date (e.g., June 4th), you can specify the excludeDates field as follows:

  ```bash
    excludeDates:
    - "* * 4 6 *"
  ```

### schedule

The `schedule` field is a cron expression that defines the schedule for restarting the target resource.

### misfirePolicy

The `misfirePolicy` field defines the behavior when a scheduled execution is missed. It can be set to "Ignore" (default) to skip missed executions, or "FireAndProceed" to execute the missed job immediately(If multiple execution windows are missed during a controller outage, the system collapses them into a single recovery execution).

### misfireDeadWindowMinutes

The `misfireDeadWindowMinutes` specifies the threshold in minutes. If the next regular execution time is closer than this window, the misfire recovery will be ignored. Default to 5 minutes if not specified.

### cron expression

The cron expression format is described below:

Field name   | Mandatory? | Allowed values  | Allowed special characters
----------   | ---------- | --------------  | --------------------------
Minutes      | Yes        | 0-59            | * / , -
Hours        | Yes        | 0-23            | * / , -
Day of month | Yes        | 1-31            | * / , - ?
Month        | Yes        | 1-12 or JAN-DEC | * / , -
Day of week  | Yes        | 0-6 or SUN-SAT  | * / , - ?

#### Special Characters

* **Asterisk ( * )**
  * The asterisk indicates that the cron expression will match for all values of the field. For example, using an asterisk in the 4th field (month) means every month.

* **Slash ( / )**
  * Slashes are used to describe increments of ranges. For example, `3-59/15` in the 1st field (minutes) means the 3rd minute of the hour and every 15 minutes thereafter. The form `*/...` is equivalent to the form `first-last/...`, which means an increment over the largest possible range of the field. The form `N/...` means starting at N and using the increment until the end of that specific range. It does not wrap around.

* **Comma ( , )**
  * Commas are used to separate items of a list. For example, using `MON,WED,FRI` in the 5th field (day of week) means Mondays, Wednesdays, and Fridays.

* **Hyphen ( - )**
  * Hyphens are used to define ranges. For example, `9-17` means every hour between 9am and 5pm inclusive.

* **Question mark ( ? )**
  * A question mark can be used instead of `*` to leave either day-of-month or day-of-week blank.

#### Predefined Schedules

You may use one of several predefined schedules in place of a cron expression:

Entry                  | Description                                | Equivalent To
-----                  | -----------                                | -------------
@yearly (or @annually) | Run once a year, midnight, Jan. 1st        | `0 0 1 1 *`
@monthly               | Run once a month, midnight, first of month | `0 0 1 * *`
@weekly                | Run once a week, midnight between Sat/Sun  | `0 0 * * 0`
@daily (or @midnight)  | Run once a day, midnight                   | `0 0 * * *`
@hourly                | Run once an hour, beginning of hour        | `0 * * * *`

#### Intervals

You can also schedule a job to execute at fixed intervals, starting at the time it's added or when cron is run. This is supported by formatting the cron spec like this:

@every `<duration>`

where `<duration>` is a string accepted by `time.ParseDuration` (<https://golang.org/pkg/time/#ParseDuration>).

For example, `@every 1h30m10s` indicates a schedule that activates after 1 hour, 30 minutes, and 10 seconds, and then every interval after that.

**Note**: The interval does not take the job runtime into account. For example, if a job takes 3 minutes to run and is scheduled to run every 5 minutes, it will have only 2 minutes of idle time between each run.

For more scheduling options, please refer to the [cron package documentation](https://godoc.org/github.com/robfig/cron).

## Contributing

Contributions are welcome! Please submit an issue or pull request to contribute to this project.

## Licensing

This project is licensed under the terms of the MIT License. See the [LICENSE](LICENSE) file for the full license text and copyright notice.
