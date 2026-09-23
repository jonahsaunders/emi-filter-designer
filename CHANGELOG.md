# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.0.0] - 2026-09-23
### Added
- Automatic synthesis of DC-input π filters (with optimum damping, an optional CM choke and 1 or 2 stages) and AC-mains filters (CM chokes, X2/Y1 capacitors, DM inductors, bleeders).
- Harmonic-by-harmonic conducted-emission simulation with a full MNA model of the LISN, the filter parasitics and the converter.
- Standards: CISPR 25 Class 1–5 (voltage method), CISPR 32 / EN 55032 A/B, CISPR 11 / EN 55011 Group 1 A/B, FCC Part 15 A/B.
- Plots of in-circuit attenuation, 50 Ω/50 Ω insertion loss and Middlebrook stability.
- Built-in library of real parts. Live Mouser (Search API v1) and DigiKey (Product Information v4) search.
- Exports: HTML design report, BOM CSV, SPICE netlist and project files.
- Windows desktop app (a single exe using an Edge app window) and a single-file WebAssembly web version for GitHub Pages.
