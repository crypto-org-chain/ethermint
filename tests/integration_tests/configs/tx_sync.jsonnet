local default = import 'default.jsonnet';

default {
  'ethermint_9000-1'+: {
    config+: {
      consensus+: {
        timeout_commit: '3s',
      },
    },
    'app-config'+: {
      'json-rpc'+: {
        'tx-sync-default-timeout': '15s',
        'tx-sync-max-timeout': '15s',
        'tx-sync-max-concurrent': 2,
      },
    },
  },
}
