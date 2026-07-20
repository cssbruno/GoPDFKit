# Brazilian Clinical Document Suite

This example creates four restrained Brazilian clinical-document designs and
six PDF artifacts at practical print sizes with the low-level
`document.Document` API:

- `pedido-exames-a5.pdf` - a two-page A5 request covering common laboratory,
  imaging, microbiology, functional, and screening exams.
- `receituario-a5.pdf` - a one-page A5 prescription using nonexistent
  demonstrative products.
- `notificacao-receita-a-demo.pdf` - monochrome artwork for yellow paper stock,
  at 200 x 60 mm.
- `notificacao-receita-b-demo.pdf` - monochrome artwork for blue paper stock,
  at 200 x 60 mm.
- `notificacao-receita-a-preview-papel-amarelo.pdf` and
  `notificacao-receita-b-preview-papel-azul.pdf` - digital previews simulating
  the corresponding colored paper.

Run the suite from the repository root:

```sh
go run ./examples/medical-document-suite
```

Generated files are written to `assets/generated/pdf/examples`.

## Safety and scope

All people, identifiers, credentials, products, addresses, and verification
values are fictional. The controlled-prescription pages intentionally contain
no valid SNCR number, no real controlled medication, zeroed credentials, and a
visible invalidity warning. They are software layout examples, not official
Anvisa forms and not documents that can be used for prescribing or dispensing.

The print-ready A/B PDFs do not paint a page background: their black artwork is
intended for the corresponding yellow or blue paper stock. The preview PDFs
only simulate that stock for on-screen inspection.

The field hierarchy follows the public physical receituary models listed on the
[Anvisa SNCR page](https://www.gov.br/anvisa/pt-br/assuntos/medicamentos/controlados/sncr/receituario-fisico/modelos-vigentes),
which should always be consulted directly for current professional use.

The data-driven A5 prescriptions are authored separately with Paper in
`examples/paper-receituario-a5`. This includes the two-page Controle Especial;
there is intentionally no competing low-level Go implementation for it.
