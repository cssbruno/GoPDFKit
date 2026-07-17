(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioInstanceModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  function identity(fragment) {
    const source = fragment?.source_identity || {};
    return {
      key: String(source.key || ''),
      instance: String(source.instance || ''),
    };
  }

  function classifyFragments(fragments) {
    const instancesByKey = new Map();
    for (const fragment of fragments || []) {
      const {key, instance} = identity(fragment);
      if (!key || !instance) continue;
      if (!instancesByKey.has(key)) instancesByKey.set(key, new Set());
      instancesByKey.get(key).add(instance);
    }

    return (fragments || []).map((fragment) => {
      const {key, instance} = identity(fragment);
      const repeated = fragment?.repeated === true;
      const expanded = !repeated && Boolean(instance) && (instance !== key || (instancesByKey.get(key)?.size || 0) > 1);
      const kind = repeated ? 'repeated' : expanded ? 'expanded' : 'authored';
      const region = String(fragment?.region || 'region');
      const name = instance || key || `fragment-${fragment?.id || '?'}`;
      const label = repeated ? `repeated ${region} · ${name}` : expanded ? `instance · ${name}` : `authored · ${name}`;
      return {
        fragment,
        kind,
        label,
        className: `is-instance is-instance-${kind}`,
        key,
        instance,
        region,
        repeated,
      };
    });
  }

  return {classifyFragments};
});
