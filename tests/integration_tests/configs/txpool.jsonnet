local default = import 'default.jsonnet';

default {
  'ethermint_9000-1'+: {
    config+: {
      consensus+: {
        timeout_commit: '5s',
      },
    },
    'app-config'+: {
      'json-rpc'+: {
        api: 'eth,net,web3,debug,txpool',
      },
    },
  },
}
