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
        'receipt-wait-timeout': '15s',
        'receipt-wait-max-timeout': '15s',
        'max-receipt-waiters': 2,
      },
    },
  },
}
