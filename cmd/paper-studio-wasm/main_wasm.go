// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

//go:build js && wasm

package main

import (
	"context"
	"errors"
	"syscall/js"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

var renderFunction js.Func

func main() {
	renderFunction = js.FuncOf(renderPage)
	engine := js.Global().Get("Object").New()
	engine.Set("formatVersion", layoutengine.WebDisplayRenderPayloadVersion)
	engine.Set("rendererVersion", layoutengine.DisplayRasterRendererVersion)
	engine.Set("render", renderFunction)
	js.Global().Set("PaperStudioWASM", engine)
	<-make(chan struct{})
}

func renderPage(_ js.Value, arguments []js.Value) any {
	promise := js.Global().Get("Promise")
	executor := js.FuncOf(func(_ js.Value, callbacks []js.Value) any {
		resolve, reject := callbacks[0], callbacks[1]
		if len(arguments) != 1 || arguments[0].Type() != js.TypeObject {
			reject.Invoke(jsError(errors.New("paper-studio-wasm: render expects one Uint8Array")))
			return nil
		}
		length := arguments[0].Get("byteLength").Int()
		if length <= 0 || length > layoutengine.WebDisplayRenderMaxPayloadBytes {
			reject.Invoke(jsError(errors.New("paper-studio-wasm: payload length is invalid")))
			return nil
		}
		payload := make([]byte, length)
		if copied := js.CopyBytesToGo(payload, arguments[0]); copied != length {
			reject.Invoke(jsError(errors.New("paper-studio-wasm: payload copy was incomplete")))
			return nil
		}
		go func() {
			artifact, err := layoutengine.RenderWebDisplayPayload(context.Background(), payload)
			if err != nil {
				reject.Invoke(jsError(err))
				return
			}
			manifestJSON, err := artifact.CanonicalManifestJSON()
			if err != nil {
				reject.Invoke(jsError(err))
				return
			}
			png := artifact.PNG()
			encoded := js.Global().Get("Uint8Array").New(len(png))
			if copied := js.CopyBytesToJS(encoded, png); copied != len(png) {
				reject.Invoke(jsError(errors.New("paper-studio-wasm: PNG copy was incomplete")))
				return
			}
			result := js.Global().Get("Object").New()
			result.Set("manifest", js.Global().Get("JSON").Call("parse", string(manifestJSON)))
			result.Set("png", encoded)
			resolve.Invoke(result)
		}()
		return nil
	})
	result := promise.New(executor)
	executor.Release()
	return result
}

func jsError(err error) js.Value {
	return js.Global().Get("Error").New(err.Error())
}
