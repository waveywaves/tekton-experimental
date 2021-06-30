# Jenkins Custom Task for Tekton

This [Tekton Custom Task](https://tekton.dev/docs/pipelines/runs/) helps Tekton to interact with Jenkins.

## Install

Install and configure `ko`.

```
ko apply -f ./config
```

This will build and install the controller on your cluster, in the namespace
`tekton-jenkins`.

## Uninstall

```
$ ko delete -f ./config
```

