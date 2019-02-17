# Traffic mirror
Mirror HTTP traffic to a main target (acting as a reverse proxy) and a runtime modifiable set of mirror targets. 

At the moment this code is a first attempt at implementing traffic mirroring (aka shadowing) to multiple targets. Most existing tools only support one additional target (the shadow copy) next to the real (production) target who's response is returned to the client.

## How to run
If you just run the command `./trafficmirror` it will bind and start listening on the default address `*:8080` and will by default reverse proxy all traffic to `http://localhost:8888`. All the defaults can be customized with command line arguments, see `trafficmirror -h` for the details.

Now that the normal flow of traffic to your application is running we can start adding mirrors. To be as flexible as possible this is not done via the command line. Instead an extra endpoint (`/targets` by default) has been exposed on the address that traffic mirror just bound to. This endpoint can be used to add,remove or list the current mirror targets. 

*IMPORTANT*
By default this endpoint will be publicly accessible. This is quick and easy, but likely not what you want. You can add password protection to it with the `-password` option. Note that the assumption here is that your application was already behind a TLS load balancer or proxy, traffic mirror doesn't support TLS traffic at the moment. Additionally you can use `-targetsAddress` to bind this `targets` endpoint to a separate address, for example `localhost:1234` (effectively limiting access to only the server itself).

## Mirror targets
Let's assume traffic mirror was started with `./trafficmirror -targetsAddress=localhost:1234`.

To add a target:

`curl -X PUT 127.0.0.1:1234/targets?url=http://firstmirror:8080`

To list the targets (in plain text):

`curl 127.0.0.1:1234/targets`

returns

```
http://firstmirror:8080
```

To remove the target again:

`curl -X DELETE 127.0.0.1:1234/targets?url=http://firstmirror:8080`

Because a request parameter can be added multiple times to a url we can also add and remove multiple targets in one go:

```
curl -X PUT "127.0.0.1:1234/targets?url=http://firstmirror:8080&url=http://secondmirror:8081"
curl 127.0.0.1:1234/targets
```

Returns

```
http://firstmirror:8080
http://secondmirror:8081
```

When password protection was enabled adapt all `curl` commands accordingly. For example if your password file contained

```
user:password
```

The list command should become:

`curl -u user:password 127.0.0.1:1234/targets`
