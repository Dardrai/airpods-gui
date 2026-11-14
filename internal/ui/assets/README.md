# Third-party artwork

These images are not original to this project and keep their own rights. AirPods and the AirPods artwork belong to
Apple.

- `airpods_<model>_left.png`, `_right.png`, `_case.png`, `airpods_max_1.png`, `airpods_max_2.png`: from
  LibrePods (https://github.com/Ripthulhu/librepods, GPL-3.0), `android/core/src/main/res-apple/drawable-nodpi`.
  Models 4 and 4 (ANC), and 5 and 5 (wireless), are the same files there, so one set of each is kept. The two Max images are the silver ones, cropped and scaled down to 320
  pixels.
- `ic_transparency.png`, `ic_adaptive.png`, `ic_noise_cancellation.png`: listening mode icons from LibrePods, by way
  of NTPods (https://github.com/Ripthulhu/ntpods, GPL-3.0).

The tray and application icon in `internal/icon` is drawn from the `ic_airpods` vector of LibrePods, GPL-3.0,
https://github.com/Ripthulhu/librepods.

To remove the AirPods images, revert the commits that added them.
