# gmock-api

A tool for Rest API test. No dependencies are required.

## usage

### compile
```bash
$ git clone ....
$ make
  # this will show make file help 
  
$ make build
  # build main.go
  
$ make dist
  # build main.go for linux arm64
  
$ make run
  # run main.go directly
  
# 
```
### edit config file
`apis.json` is a sample config file.
```json
{
  "apis": [
    {
      "paths":         ["/info", "/status"],
      "methods":       ["GET"],
      "mock_response": {
        "result": {
          "name": "magic-mock-api",
          "version": ".0.9.5",
          "description": "mock rest api tool"
        },
        "status_code": 200
      }
    },
    {
      "paths":         ["/register"],
      "methods":       ["POST", "PUT"],
      "mock_response": {
        "result": {
          "act_type": 1,
          "status": "success",
          "register_id": "a99f391852caf97496e7a5ad27a7f295ecc194061b490985959472f3da3d00fb"
        },
        "status_code": 200
      }
    },
    ...
  ]
}
```

### test 
run with `--help` to show usage.
```bash
$ ./build/bin/mockapi --help
Usage of ./build/bin/mockapi:
  -conf string
    	json config file (default "apis.json")
  -debug
    	debug mode
  -port int
    	agent server port (default 7001)
```

use your favor rest tool to access `http://localhost:700/info`
```bash
$ curl -s http://localhost:7001/info | jq
  # get sample

$ curl -s -H "Content-Type: application/json; charset=UTF-8" -d '{"agent_id": "tony_test_id_000", "agent_version": "1.0.1"}' http://localhost:7001/register | jq
  # post sample
```

Happy Hacking!
