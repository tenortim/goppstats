# Changelog

## v0.20 - Tue Oct 31 09:41:21 2023 -0700

> [!IMPORTANT]
> The config rewrite is a breaking change which will require
> a manual update of the configuration file post-upgrade.

### Major changes

- Major config rewrite
  - This config change means older config files must be updated to be compatible
  - Renamed back end names (removed "_plugin")
  - Removed hacky "processor_args" inherited from the Python collector
  - Added config stanzas for InfluxDB and Prometheus
  - Added missing "lookup_export_ids" setting

## v0.19 - Mon Oct 30 15:44:38 2023 -0700

### Major changes

- Added config version checking
  - Upcoming changes will break the config file format.
  - Added a version check to avoid unexplained breakage.

## v0.18 - Sat Oct 28 15:18:39 2023 -0700

### Security

- Updated all dependencies to latest versions

### Bug fixes

- Restored missing "isilon" namespace from stats
  - In the switch to using our own Collector interface rather than the pre-canned gauges, the metric names lost their namespace prefix.

## v0.17 - Thu Oct 26 13:26:06 2023 -0700

### Bug fixes

- Reworked Prometheus support
  - The initial Prometheus support had a major issue where stats that had been collected at least once, but which did not appear in the current collection cycle were still exposed via the `/metrics` endpoint
  - Completely rewrote the code. Advantages of the rewritten code include
    - the collector now correctly exports the original metric timestamp for each metric, and
    - the collector now expires metrics based on the metadata that defines how long they are value so stale metrics correctly disappear from the `/metrics` endpoint

## v0.16 - Mon Oct 2 12:56:59 2023 -0700

### New features

- Added support for NFS export id to path lookup
  - Optionally enabled via the config file.
  - If enabled, it is necessary to grant readonly NFS API access to theconfigured stats user.

## v0.15 - Mon Sep 18 14:18:14 2023 -0700

### New features

- Added support for hardcoding the HTTP SD listen addr

### Bug fixes

- Fixed HTTP SD output

## v0.14 - Wed Aug 30 15:53:42 2023 -0700

### New features

- Added support for Prometheus HTTP SD discovery

## v0.13 - Tue Aug 1 16:57:57 2023 -0700

### Bug fixes

 Fixed http routing with multple servers

## v0.12 - Tue Aug 1 16:28:19 2023 -0700

### Bug fixes

- Fixed prometheus plugin argument handling

## v0.11 - Fri Jul 28 08:23:46 2023 -0700

### Major changes

- example configuration filename changed to match tool name

### Bug fixes

- Fixed cluster name breakage introduced by prom work

## v0.10 - Thu Jul 27 10:16:23 2023 -0700

Version bump to sync with the current version of gostats

### New features

- Add Prometheus support
- Add support for authenticated InfluxDB connection

## v0.01 - Thu May 12 08:26:16 2022 -0700

### Initial release of the Golang OneFS stats collector
