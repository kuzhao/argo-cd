# Overview

This guide is for developers who have Argo CD installed for them and are managing applications.

When the `argocd` CLI accesses Kubernetes directly, it prints the target Kubernetes server URL or IP address to stderr before accessing the cluster.
This includes cluster registration, administrative commands, core mode, and port forwarding.
Command results on stdout remain unchanged.

> [!NOTE]
> Please make sure you've completed the [getting started guide](../getting_started.md). 
