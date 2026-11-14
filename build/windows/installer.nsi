; Per-user Windows installer for the tray app and the airpods command line tool. Needs no admin
; rights and is not signed. build/fyne-cross.sh installer defines VERSION, NUMERIC_VERSION, ROOT,
; ICON and OUTFILE.
;
; It installs both by default. Unticking the app leaves the command line tool alone, and the
; other way round. An upgrade starts from what the last install chose.
;
; Installer switches:   /S  /CLIONLY  /NOCLI  /NOPATH  /AUTOSTART  /DESKTOP  /RELAUNCH
;                       /D=path (last, not quoted)
; /CLIONLY installs the command line tool without the app, and /NOCLI the app without it.
; /NOPATH leaves the user's PATH alone.
; /RELAUNCH starts the app again once a silent install is done: the in-app updater uses it.
; Uninstaller switches: /S  /REMOVESETTINGS  _?=path (wait for it, last)

Unicode true
SetCompressor /SOLID lzma
RequestExecutionLevel user
ManifestDPIAware true

!include MUI2.nsh
!include LogicLib.nsh
!include FileFunc.nsh
!include Sections.nsh
!include WinVer.nsh
!include x64.nsh

!define APP "AirPods"
; The name before 0.3.1: the install folder keeps it, so upgrades stay in place, and its
; shortcuts are removed.
!define OLD_APP "AirPods Tray"
!define EXE "airpods-tray.exe"
; The command line tool. Its hidden --installer-path switch puts its own folder on the user's
; PATH or takes it out (cmd/airpods/installpath.go); a test there keeps the switch equal.
!define CLI_EXE "airpods.exe"
!define CLI_PATH_SWITCH "--installer-path"
!define PUBLISHER "airpods-gui contributors"
!define UNINSTALLER "uninstall.exe"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\AirPodsTray"
; The same entry as internal/autostart; a test there keeps them equal.
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define RUN_VALUE "AirPodsTray"
; Where the app keeps its settings: Fyne's preferences and the per-device choices. The cache
; directory only holds the single-instance socket.
!define FYNE_DIR "$APPDATA\fyne"
!define PREFS_DIR "${FYNE_DIR}\io.github.dardrai.airpods-gui"
!define CHOICES_DIR "$APPDATA\airpods-tray"
!define CACHE_DIR "$LOCALAPPDATA\airpods-tray"
; The app names its toast sender here on every start (internal/desktopnote); a test keeps it
; equal. The old key is what an earlier build registered under the app id.
!define TOAST_KEY "Software\Classes\AppUserModelId\Dardrai.AirPodsTray"
!define OLD_TOAST_KEY "Software\Classes\AppUserModelId\io.github.dardrai.airpods-gui"
; Values under UNINSTALL_KEY that record what was installed: 1 or 0.
!define CHOICE_APP "InstalledApp"
!define CHOICE_CLI "InstalledCommandLine"
!define CHOICE_PATH "CommandLineOnPath"

Name "${APP}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\${OLD_APP}"
InstallDirRegKey HKCU "${UNINSTALL_KEY}" "InstallLocation"
BrandingText "${APP} ${VERSION}"
ShowInstDetails nevershow
ShowUninstDetails nevershow

!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_NODESC
!define MUI_LICENSEPAGE_TEXT_BOTTOM "This program is free software under the GNU General Public License, version 3."
!define MUI_LICENSEPAGE_BUTTON "$(^NextBtn)"
!insertmacro MUI_PAGE_LICENSE "${ROOT}/LICENSE"
!define MUI_COMPONENTSPAGE_TEXT_TOP "Both are installed unless you untick one. Untick the ${APP} app to install the command line tool alone."
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE ComponentsLeave
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Run ${APP} now"
!define MUI_PAGE_CUSTOMFUNCTION_SHOW FinishShow
!insertmacro MUI_PAGE_FINISH

!define MUI_COMPONENTSPAGE_TEXT_TOP "Your settings are kept unless you choose to remove them."
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_COMPONENTS
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

VIProductVersion "${NUMERIC_VERSION}"
VIFileVersion "${NUMERIC_VERSION}"
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductName" "${APP}"
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=${LANG_ENGLISH} "CompanyName" "${PUBLISHER}"
VIAddVersionKey /LANG=${LANG_ENGLISH} "LegalCopyright" "${PUBLISHER}, GPL-3.0"
VIAddVersionKey /LANG=${LANG_ENGLISH} "FileDescription" "${APP} Setup"
VIAddVersionKey /LANG=${LANG_ENGLISH} "FileVersion" "${VERSION}"

; WaitForRelease sets $R0 to 1 once $INSTDIR holds no running executable, polling for up to
; 5 seconds, because a process still holds its file for a moment after it stops answering.
!macro WAIT_FOR_RELEASE_FUNC PREFIX
Function ${PREFIX}WaitForRelease
  Push $0
  Push $1
  StrCpy $R0 0
  StrCpy $0 0
  ${Do}
    ${IfNot} ${FileExists} "$INSTDIR\${EXE}"
      StrCpy $R0 1
      ${Break}
    ${EndIf}
    ClearErrors
    FileOpen $1 "$INSTDIR\${EXE}" a
    ${IfNot} ${Errors}
      FileClose $1
      StrCpy $R0 1
      ${Break}
    ${EndIf}
    Sleep 250
    IntOp $0 $0 + 1
  ${LoopUntil} $0 >= 20
  Pop $1
  Pop $0
FunctionEnd

; CloseApp ends a running instance. $R1 is an executable that understands --quit. It asks
; first. Only an instance that stays, and still holds the file, is then ended by taskkill,
; which reaches every process of that name for this user. The installer gives up if even
; that fails, rather than leave a mix of old and new files.
Function ${PREFIX}CloseApp
  ${If} ${FileExists} "$R1"
    ExecWait '"$R1" --quit' $0
  ${EndIf}
  Call ${PREFIX}WaitForRelease
  ${While} $R0 != 1
    nsExec::Exec 'taskkill /IM ${EXE}'
    Pop $0
    Call ${PREFIX}WaitForRelease
    ${If} $R0 != 1
      nsExec::Exec 'taskkill /F /IM ${EXE}'
      Pop $0
      Call ${PREFIX}WaitForRelease
    ${EndIf}
    ${If} $R0 != 1
      MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "${APP} is still running. Close it from its tray icon, then choose Retry." /SD IDCANCEL IDRETRY retry
      SetErrorLevel 3
      Abort "${APP} is still running."
      retry:
    ${EndIf}
  ${EndWhile}
FunctionEnd
!macroend
!insertmacro WAIT_FOR_RELEASE_FUNC ""
!insertmacro WAIT_FOR_RELEASE_FUNC "un."

; CLOSE_RUNNING_APP ends a running app before its file is replaced or removed. The new
; executable can ask any older one to quit, wherever that one was started from.
!macro CLOSE_RUNNING_APP
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File "/oname=${EXE}" "${ROOT}/dist/${EXE}"
  StrCpy $R1 "$PLUGINSDIR\${EXE}"
  Call CloseApp
!macroend

Section "${APP} app (tray and settings window)" SecApp
  !insertmacro CLOSE_RUNNING_APP
  SetOutPath "$INSTDIR"
  File "${ROOT}/dist/${EXE}"
  CreateShortcut "$SMPROGRAMS\${APP}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\${EXE}" 0
SectionEnd

Section /o "Start the app with Windows" SecAutostart
SectionEnd

Section /o "Desktop shortcut for the app" SecDesktop
SectionEnd

Section "Command line tool (airpods)" SecCli
  SetOutPath "$INSTDIR"
  File "/oname=${CLI_EXE}" "${ROOT}/dist/airpods-windows-amd64.exe"
SectionEnd

Section "Add the command line tool to PATH" SecPath
SectionEnd

; Unticking a box on an upgrade removes what an earlier install set up.
Section "-Apply" SecApply
  SetOutPath "$INSTDIR"
  File "/oname=LICENSE.txt" "${ROOT}/LICENSE"
  WriteUninstaller "$INSTDIR\${UNINSTALLER}"
  Delete "$SMPROGRAMS\${OLD_APP}.lnk"
  Delete "$DESKTOP\${OLD_APP}.lnk"

  ${If} ${SectionIsSelected} ${SecApp}
    StrCpy $R2 1
  ${Else}
    StrCpy $R2 0
    ${If} ${FileExists} "$INSTDIR\${EXE}"
      !insertmacro CLOSE_RUNNING_APP
      SetOutPath "$INSTDIR"
    ${EndIf}
    Delete "$INSTDIR\${EXE}"
    Delete "$SMPROGRAMS\${APP}.lnk"
  ${EndIf}
  ${If} $R2 == 1
  ${AndIf} ${SectionIsSelected} ${SecAutostart}
    WriteRegStr HKCU "${RUN_KEY}" "${RUN_VALUE}" '"$INSTDIR\${EXE}" --tray'
  ${Else}
    DeleteRegValue HKCU "${RUN_KEY}" "${RUN_VALUE}"
  ${EndIf}
  ${If} $R2 == 1
  ${AndIf} ${SectionIsSelected} ${SecDesktop}
    CreateShortcut "$DESKTOP\${APP}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\${EXE}" 0
  ${Else}
    Delete "$DESKTOP\${APP}.lnk"
  ${EndIf}

  ; $R3 is whether the command line tool is installed, and $R4 whether its folder is on PATH.
  StrCpy $R3 0
  StrCpy $R4 0
  ${If} ${SectionIsSelected} ${SecCli}
    StrCpy $R3 1
    ${If} ${SectionIsSelected} ${SecPath}
      StrCpy $R4 1
      StrCpy $R5 add
    ${Else}
      StrCpy $R5 remove
    ${EndIf}
    Call ApplyPath
  ${ElseIf} ${FileExists} "$INSTDIR\${CLI_EXE}"
    StrCpy $R5 remove
    Call ApplyPath
    Delete "$INSTDIR\${CLI_EXE}"
  ${EndIf}

  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${APP}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "${PUBLISHER}"
  ${If} $R2 == 1
    WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\${EXE},0"
  ${Else}
    WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\${UNINSTALLER},0"
  ${EndIf}
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\${UNINSTALLER}"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '"$INSTDIR\${UNINSTALLER}" /S'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
  ; What this install chose, for the next one to start from.
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "${CHOICE_APP}" $R2
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "${CHOICE_CLI}" $R3
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "${CHOICE_PATH}" $R4
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "EstimatedSize" "$0"
SectionEnd

; ApplyPath runs the command line tool's own switch, with add or remove in $R5. A failure
; leaves the install as it is: the tool still works by its full path.
Function ApplyPath
  nsExec::Exec '"$INSTDIR\${CLI_EXE}" ${CLI_PATH_SWITCH} $R5'
  Pop $0
  ${If} $0 != 0
  ${AndIf} $R5 == add
    StrCpy $R4 0
    MessageBox MB_OK|MB_ICONEXCLAMATION "The folder of the airpods command could not be added to PATH. Add it yourself to run airpods from any terminal:$\r$\n$\r$\n$INSTDIR" /SD IDOK
  ${EndIf}
FunctionEnd

; At least one of the two has to be installed.
Function ComponentsLeave
  ${IfNot} ${SectionIsSelected} ${SecApp}
  ${AndIfNot} ${SectionIsSelected} ${SecCli}
    MessageBox MB_OK|MB_ICONEXCLAMATION "Choose the ${APP} app, the command line tool, or both."
    Abort
  ${EndIf}
FunctionEnd

; Without the app there is nothing to run.
Function FinishShow
  ${IfNot} ${SectionIsSelected} ${SecApp}
    SendMessage $mui.FinishPage.Run ${BM_SETCHECK} ${BST_UNCHECKED} 0
    ShowWindow $mui.FinishPage.Run ${SW_HIDE}
  ${EndIf}
FunctionEnd

Function .onInstSuccess
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/RELAUNCH" $1
  ${IfNot} ${Errors}
  ${AndIf} ${FileExists} "$INSTDIR\${EXE}"
    Exec '"$INSTDIR\${EXE}"'
  ${EndIf}
FunctionEnd

; SetChoice selects or unselects a section from a 0 or 1 that the last install recorded.
!macro SET_CHOICE VALUE SECTION
  ClearErrors
  ReadRegDWORD $0 HKCU "${UNINSTALL_KEY}" "${VALUE}"
  ${IfNot} ${Errors}
    ${If} $0 == 0
      !insertmacro UnselectSection ${SECTION}
    ${Else}
      !insertmacro SelectSection ${SECTION}
    ${EndIf}
  ${EndIf}
!macroend

Function .onInit
  ${IfNot} ${RunningX64}
  ${OrIfNot} ${AtLeastWin10}
    MessageBox MB_OK|MB_ICONSTOP "${APP} needs 64-bit Windows 10 or newer." /SD IDOK
    SetErrorLevel 4
    Abort
  ${EndIf}
  ; An upgrade starts from what is set up now, so a silent one changes nothing. An install
  ; from before the command line tool recorded no choice, and gets the tool and its PATH entry.
  !insertmacro SET_CHOICE "${CHOICE_APP}" ${SecApp}
  !insertmacro SET_CHOICE "${CHOICE_CLI}" ${SecCli}
  !insertmacro SET_CHOICE "${CHOICE_PATH}" ${SecPath}
  ClearErrors
  ReadRegStr $0 HKCU "${RUN_KEY}" "${RUN_VALUE}"
  ${IfNot} ${Errors}
    !insertmacro SelectSection ${SecAutostart}
  ${EndIf}
  ${If} ${FileExists} "$DESKTOP\${APP}.lnk"
  ${OrIf} ${FileExists} "$DESKTOP\${OLD_APP}.lnk"
    !insertmacro SelectSection ${SecDesktop}
  ${EndIf}
  ${GetParameters} $0
  ClearErrors
  StrCpy $2 0
  ${GetOptions} $0 "/CLIONLY" $1
  ${IfNot} ${Errors}
    StrCpy $2 1
    !insertmacro UnselectSection ${SecApp}
    !insertmacro SelectSection ${SecCli}
  ${EndIf}
  ClearErrors
  ${GetOptions} $0 "/NOCLI" $1
  ${IfNot} ${Errors}
    ${If} $2 == 1
      MessageBox MB_OK|MB_ICONSTOP "/CLIONLY and /NOCLI together leave nothing to install." /SD IDOK
      SetErrorLevel 2
      Abort
    ${EndIf}
    !insertmacro UnselectSection ${SecCli}
    !insertmacro SelectSection ${SecApp}
  ${EndIf}
  ClearErrors
  ${GetOptions} $0 "/NOPATH" $1
  ${IfNot} ${Errors}
    !insertmacro UnselectSection ${SecPath}
  ${EndIf}
  ClearErrors
  ${GetOptions} $0 "/AUTOSTART" $1
  ${IfNot} ${Errors}
    !insertmacro SelectSection ${SecAutostart}
  ${EndIf}
  ClearErrors
  ${GetOptions} $0 "/DESKTOP" $1
  ${IfNot} ${Errors}
    !insertmacro SelectSection ${SecDesktop}
  ${EndIf}
  ; Recorded choices that name neither cannot come from this installer; start from both.
  ${IfNot} ${SectionIsSelected} ${SecApp}
  ${AndIfNot} ${SectionIsSelected} ${SecCli}
    !insertmacro SelectSection ${SecApp}
    !insertmacro SelectSection ${SecCli}
  ${EndIf}
FunctionEnd

Section "un.${APP}" UnSecApp
  SectionIn RO
  StrCpy $R1 "$INSTDIR\${EXE}"
  Call un.CloseApp
  Delete "$INSTDIR\${EXE}"
  ${If} ${FileExists} "$INSTDIR\${CLI_EXE}"
    nsExec::Exec '"$INSTDIR\${CLI_EXE}" ${CLI_PATH_SWITCH} remove'
    Pop $0
    Delete "$INSTDIR\${CLI_EXE}"
  ${EndIf}
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\${UNINSTALLER}"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${APP}.lnk"
  Delete "$DESKTOP\${APP}.lnk"
  Delete "$SMPROGRAMS\${OLD_APP}.lnk"
  Delete "$DESKTOP\${OLD_APP}.lnk"
  DeleteRegValue HKCU "${RUN_KEY}" "${RUN_VALUE}"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  DeleteRegKey HKCU "${TOAST_KEY}"
  DeleteRegKey HKCU "${OLD_TOAST_KEY}"
  Delete "${CACHE_DIR}\notify.png"
  RMDir "${CACHE_DIR}"
SectionEnd

Section /o "un.Remove my settings" UnSecSettings
  RMDir /r "${PREFS_DIR}"
  RMDir "${FYNE_DIR}"
  RMDir /r "${CHOICES_DIR}"
  RMDir /r "${CACHE_DIR}"
SectionEnd

Function un.onInit
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/REMOVESETTINGS" $1
  ${IfNot} ${Errors}
    !insertmacro SelectSection ${UnSecSettings}
  ${EndIf}
FunctionEnd
