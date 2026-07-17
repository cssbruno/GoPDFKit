(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioProvenanceModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  function normalize(payload) {
    const source = payload && typeof payload === 'object' ? payload : {};
    return {
      bindings: Array.isArray(source.bindings) ? source.bindings.filter(item => item && typeof item === 'object') : [],
      styleTokens: Array.isArray(source.style_tokens) ? source.style_tokens.filter(item => item && typeof item === 'object') : [],
    };
  }

  function forFragments(payload, fragments) {
    const provenance = normalize(payload);
    const keys = new Set((fragments || []).map(fragment => String(fragment?.source_identity?.key || '')).filter(Boolean));
    const matches = item => !keys.size || !item.node || keys.has(String(item.node));
    return {
      bindings: provenance.bindings.filter(matches),
      styleTokens: provenance.styleTokens.filter(matches),
    };
  }

  function bindingLabel(binding) {
    return `${binding.node || 'anonymous'} ← ${binding.path || 'unbound'}${binding.kind ? ` · ${binding.kind}` : ''}`;
  }

  function tokenLabel(token) {
    const chain = (token.token_chain || []).map(step => `${step.theme || '?'}:${step.token || '?'}`).join(' → ');
    return `${token.node || 'anonymous'} · ${token.property || 'style'} ← ${token.theme || '?'}:${token.token || '?'} = ${token.value || '?'}${chain ? ` · ${chain}` : ''}`;
  }

  return Object.freeze({normalize, forFragments, bindingLabel, tokenLabel});
});
