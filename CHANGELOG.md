## 0.6.0 / 2023-06-06

* [FEATURE] Allow timestamps from metrics #167
* [FEATURE] Support Value Conversions #172

## 0.5.0 / 2022-07-03

Breaking Change:

The exporter config file format has changed. It now supports multiple modules
to scrape different endpoints.

* [FEATURE] Support custom valuetype #145
* [FEATURE] Support modules configuration #146
* [FEATURE] Accept non-2xx HTTP status #161

## 0.4.0 / 2022-01-15

* [FEATURE] Add support for HTTP POST body content #123

## 0.3.0 / 2021-02-12

:warning: Backward incompatible configuration with previous versions.
* [CHANGE] Migrate JSONPath library [#74](https://github.com/prometheus-community/json_exporter/pull/74)
* [CHANGE] Add TLS metrics support [#68](https://github.com/prometheus-community/json_exporter/pull/68)

## 0.2.0 / 2020-11-03

* [CHANGE] This release is complete refactoring [#49](https://github.com/prometheus-community/json_exporter/pull/49)
* [BUGFIX] Fix unchecked call to io.Copy [#57](https://github.com/prometheus-community/json_exporter/pull/57)

## 0.1.0 / 2020-07-27

Initial prometheus-community release.
