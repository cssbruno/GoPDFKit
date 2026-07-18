(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.PaperStudioViewportModel = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  const minZoom = 0.1;
  const maxZoom = 4;
  const minDPI = 36;
  const maxDPI = 600;

  function clamp(value, minimum, maximum) {
    return Math.max(minimum, Math.min(maximum, value));
  }

  function zoom(value) {
    return clamp(Math.round(Number(value) * 100) / 100, minZoom, maxZoom);
  }

  function fitZoom({pageWidth, pageHeight, viewportWidth, viewportHeight, paddingInline = 0, paddingBlock = 0, mode = 'page'}) {
    const values = [pageWidth, pageHeight, viewportWidth, viewportHeight, paddingInline, paddingBlock].map(Number);
    if (values.some((value) => !Number.isFinite(value)) || pageWidth <= 0 || pageHeight <= 0 || viewportWidth <= 0 || viewportHeight <= 0) return 1;
    const availableWidth = Math.max(1, viewportWidth - Math.max(0, paddingInline));
    const availableHeight = Math.max(1, viewportHeight - Math.max(0, paddingBlock));
    const widthScale = availableWidth / pageWidth;
    const scale = mode === 'width' ? widthScale : Math.min(widthScale, availableHeight / pageHeight);
    // Round down so a page that fits mathematically cannot overflow because of
    // subpixel layout or scrollbar rounding in the browser.
    return zoom(Math.floor(scale * 100) / 100);
  }

  function deviceScale(value) {
    const ratio = Number(value);
    return Number.isFinite(ratio) && ratio > 0 ? clamp(ratio, 0.5, 4) : 1;
  }

  function renderDPI(zoomValue, devicePixelRatio) {
    return Math.round(clamp(72 * zoom(zoomValue) * deviceScale(devicePixelRatio), minDPI, maxDPI));
  }

  function previewDPI(zoomValue, devicePixelRatio) {
    const target = renderDPI(zoomValue, devicePixelRatio);
    return Math.round(clamp(target * 2 / 3, minDPI, target));
  }

  return Object.freeze({minZoom, maxZoom, minDPI, maxDPI, zoom, fitZoom, deviceScale, renderDPI, previewDPI});
});
