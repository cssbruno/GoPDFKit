(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioSyntaxModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  const escapeHTML = (value) => value.replace(/[&<>]/g, character => ({'&': '&amp;', '<': '&lt;', '>': '&gt;'}[character]));
  const span = (kind, value) => `<span class="syntax-${kind}">${escapeHTML(value)}</span>`;
  const isIdentifierStart = character => /[A-Za-z_]/.test(character || '');
  const isIdentifierPart = character => /[A-Za-z0-9_-]/.test(character || '');
  const isNumberStart = (source, index) => /\d/.test(source[index] || '') || ((source[index] === '-' || source[index] === '+') && /\d/.test(source[index + 1] || ''));

  function highlight(source) {
    if (typeof source !== 'string') throw new TypeError('Paper source must be a string');
    let output = '';
    let index = 0;
    let lineContent = false;
    while (index < source.length) {
      const character = source[index];
      if (character === '\n') {
        output += '\n';
        index += 1;
        lineContent = false;
        continue;
      }
      if (/\s/.test(character)) {
        output += character;
        index += 1;
        continue;
      }
      if (character === '#') {
        const end = source.indexOf('\n', index);
        const stop = end < 0 ? source.length : end;
        output += span('comment', source.slice(index, stop));
        index = stop;
        lineContent = true;
        continue;
      }
      if (character === '"' || character === "'") {
        const quote = character;
        let end = index + 1;
        while (end < source.length) {
          if (source[end] === '\\') end += 2;
          else if (source[end++] === quote) break;
          else if (source[end - 1] === '\n') break;
        }
        output += span('string', source.slice(index, end));
        index = end;
        lineContent = true;
        continue;
      }
      if (character === '@') {
        let end = index + 1;
        while (isIdentifierPart(source[end])) end += 1;
        output += span('identity', source.slice(index, end));
        index = end;
        lineContent = true;
        continue;
      }
      if (isNumberStart(source, index)) {
        const match = source.slice(index).match(/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:pt|px|mm|cm|in|pc|em|rem|%|deg)?/i);
        output += span('number', match[0]);
        index += match[0].length;
        lineContent = true;
        continue;
      }
      if (isIdentifierStart(character)) {
        let end = index + 1;
        while (isIdentifierPart(source[end])) end += 1;
        const value = source.slice(index, end);
        let next = end;
        while (source[next] === ' ' || source[next] === '\t') next += 1;
        let kind = 'name';
        if (/^(true|false|null|none)$/i.test(value)) kind = 'literal';
        else if (!lineContent && source[next] === '@') kind = 'keyword';
        else if (source[next] === ':') kind = 'property';
        output += span(kind, value);
        index = end;
        lineContent = true;
        continue;
      }
      output += span('punctuation', character);
      index += 1;
      lineContent = true;
    }
    return output;
  }

  return Object.freeze({highlight});
});
