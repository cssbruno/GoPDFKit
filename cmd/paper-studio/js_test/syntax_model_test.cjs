const test = require('node:test');
const assert = require('node:assert/strict');

const model = require('../web/syntax-model.js');

function textFromHighlight(value) {
  return value.replace(/<[^>]+>/g, '').replaceAll('&lt;', '<').replaceAll('&gt;', '>').replaceAll('&amp;', '&');
}

test('colors Paper structure, identities, properties, values, and comments', () => {
  const source = 'document @report:\n  title: "A & B"\n  enabled: true\n  width: 72pt # exact\n';
  const highlighted = model.highlight(source);
  assert.match(highlighted, /syntax-keyword">document/);
  assert.match(highlighted, /syntax-identity">@report/);
  assert.match(highlighted, /syntax-property">title/);
  assert.match(highlighted, /syntax-string">&quot;A &amp; B&quot;|syntax-string">"A &amp; B"/);
  assert.match(highlighted, /syntax-literal">true/);
  assert.match(highlighted, /syntax-number">72pt/);
  assert.match(highlighted, /syntax-comment"># exact/);
  assert.equal(textFromHighlight(highlighted), source);
});

test('escapes source markup without changing its visible text', () => {
  const source = '  text: "<unsafe>"\n';
  const highlighted = model.highlight(source);
  assert.doesNotMatch(highlighted, /<unsafe>/);
  assert.equal(textFromHighlight(highlighted), source);
  assert.throws(() => model.highlight(null), /must be a string/);
});
