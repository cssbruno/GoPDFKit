# A5 Receituários in Paper

This example defines a restrained A5 prescription as data-driven `.paper`
source. It uses the same fictional content and safety labeling as the Go
version in `examples/medical-document-suite`.

From the repository root:

```sh
go run ./cmd/paper check \
  --assets examples/paper-receituario-a5/assets.json \
  --data examples/paper-receituario-a5/example.json \
  examples/paper-receituario-a5/receituario.paper

go run ./cmd/paper render \
  --assets examples/paper-receituario-a5/assets.json \
  --data examples/paper-receituario-a5/example.json \
  -o output/pdf/receituario-a5-paper.pdf \
  examples/paper-receituario-a5/receituario.paper
```

`A5` is a named Paper page size (148 x 210 mm). All patient, professional,
clinic, product, and verification data in this fixture is fictional. The PDF
is a layout example, not a prescription that can be used clinically.

Both prescription sources use an anonymous schema and root-relative bindings,
for example `bind: "patient.name"`. Their declarations are direct typed lines
such as `string clinic`; the ordinary prescription also demonstrates reusable
`object Patient:` and `object Medication:` declarations with `Patient patient`
and `list Medication medications:` fields.

## Controle Especial em duas vias

The Controle Especial example is also authored in Paper. It deliberately uses
an open, unruled prescription area and only the separators needed for the
issuer, patient, signature, buyer, and supplier groups.

```sh
go run ./cmd/paper check \
  --assets examples/paper-receituario-a5/assets.json \
  --data examples/paper-receituario-a5/controle-especial.json \
  examples/paper-receituario-a5/controle-especial.paper

go run ./cmd/paper render \
  --assets examples/paper-receituario-a5/assets.json \
  --data examples/paper-receituario-a5/controle-especial.json \
  -o output/pdf/receita-controle-especial-a5-demo.pdf \
  examples/paper-receituario-a5/controle-especial.paper
```

The explicit Paper `page-break` separates the pharmacy and patient copies in
the same two-page A5 PDF.

### Coleção de temas

`controle-especial.paper` declares five native Paper themes with inherited,
typed design tokens:

- `grafite` - neutral monochrome print treatment.
- `azul-clinico` - cool clinical blue hierarchy.
- `verde-institucional` - restrained institutional green.
- `vinho-classico` - warmer classic typography and burgundy accents.
- `areia-editorial` - compact editorial scale with earth-tone accents.

Render all five two-page variants from the same Paper source and JSON fixture:

```sh
examples/paper-receituario-a5/render-controle-especial-themes.sh
```

The script changes only the selected Paper theme in a temporary source file;
all layout, pagination, binding, and PDF generation still run through the
Paper compiler. Outputs are written to
`output/pdf/controle-especial-temas/`.
