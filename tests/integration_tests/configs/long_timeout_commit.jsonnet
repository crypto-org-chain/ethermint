local default = import 'default.jsonnet';

default {
  'ethermint-9000'+: {
    config+: {
      consensus+: {
        timeout_commit: '5s',
      },
    },
  },
}
