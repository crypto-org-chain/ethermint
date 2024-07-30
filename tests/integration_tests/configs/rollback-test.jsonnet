local config = import 'default.jsonnet';

config {
  'ethermint-9000'+: {
    validators: super.validators[0:1] + [{
      name: 'fullnode',
    }],
  },
}
