package main

import (
    "context"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/passbolt/go-passbolt/api"
    "github.com/passbolt/go-passbolt/helper"
    "github.com/pquerna/otp/totp"

    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/app"
    "fyne.io/fyne/v2/container"
    "fyne.io/fyne/v2/dialog"
    "fyne.io/fyne/v2/layout"
    "fyne.io/fyne/v2/theme"
    "fyne.io/fyne/v2/widget"
)

// --------------------
// Environment helpers
// --------------------

func mustGetEnv(key string) string {
    v := strings.TrimSpace(os.Getenv(key))
    if v == "" {
        log.Fatalf("Environment variable %s is required", key)
    }
    return v
}

func loadCredentials() (serverURL, privKey, passphrase, totpSecret string) {
    serverURL = mustGetEnv("PASSBOLT_URL")
    passphrase = mustGetEnv("PASSBOLT_PASSPHRASE")
    totpSecret = os.Getenv("PASSBOLT_TOTP_SECRET")

    if key := os.Getenv("PASSBOLT_PRIVATE_KEY"); key != "" {
        privKey = key
    } else {
        keyPath := mustGetEnv("PASSBOLT_PRIVATE_KEY_FILE")
        b, err := ioutil.ReadFile(keyPath)
        if err != nil {
            log.Fatalf("reading private key file: %v", err)
        }
        privKey = string(b)
    }
    return
}

// ------------------------------------------------
// MFA helpers
// ------------------------------------------------

func verifyTOTP(ctx context.Context, c *api.Client, code string) (http.Cookie, error) {
    resp := api.MFAChallengeResponse{TOTP: code}
    raw, _, err := c.DoCustomRequestAndReturnRawResponse(ctx, "POST", "mfa/verify/totp.json", "v2", resp, nil)
    if err != nil {
        return http.Cookie{}, err
    }
    for _, ck := range raw.Cookies() {
        if ck.Name == "passbolt_mfa" {
            return *ck, nil
        }
    }
    return http.Cookie{}, fmt.Errorf("passbolt_mfa cookie not returned – verification failed")
}

func autoTOTPCallback(secret string) func(context.Context, *api.Client, *api.APIResponse) (http.Cookie, error) {
    return func(ctx context.Context, c *api.Client, _ *api.APIResponse) (http.Cookie, error) {
        code, err := totp.GenerateCode(secret, time.Now())
        if err != nil {
            return http.Cookie{}, err
        }
        return verifyTOTP(ctx, c, code)
    }
}

func interactiveMFACallback(app fyne.App) func(context.Context, *api.Client, *api.APIResponse) (http.Cookie, error) {
    return func(ctx context.Context, c *api.Client, _ *api.APIResponse) (http.Cookie, error) {
        codeCh := make(chan string)

        fyne.Do(func() {
            win := app.NewWindow("Passbolt – MFA")
            entry := widget.NewEntry()
            entry.SetPlaceHolder("123456")
            okBtn := widget.NewButton("OK", func() {
                codeCh <- strings.TrimSpace(entry.Text)
                win.Close()
            })
            win.SetContent(container.New(layout.NewVBoxLayout(), widget.NewLabel("Enter 6‑digit TOTP code:"), entry, okBtn))
            win.Show()
        })

        code := <-codeCh
        return verifyTOTP(ctx, c, code)
    }
}

func configureMFA(ctx context.Context, c *api.Client, totpSecret string, app fyne.App) {
    if totpSecret != "" {
        c.MFACallback = autoTOTPCallback(totpSecret)
        return
    }
    c.MFACallback = interactiveMFACallback(app)
}

// --------------------
// Client initialisation
// --------------------

func newClient(ctx context.Context, app fyne.App) (*api.Client, error) {
    url, privKey, pw, totp := loadCredentials()

    client, err := api.NewClient(nil, "", url, privKey, pw)
    if err != nil {
        return nil, fmt.Errorf("new client: %w", err)
    }

    configureMFA(ctx, client, totp, app)

    if err := client.Login(ctx); err != nil {
        return nil, err
    }
    return client, nil
}

// --------------------
// GUI helpers
// --------------------

type uiState struct {
    resources       []api.Resource
    filtered        []api.Resource
    list            *widget.List
    searchEntry     *widget.Entry
    passEntry       *widget.Entry
    nameLabel       *widget.Label
    userLabel       *widget.Label
    uriLabel        *widget.Label
    clipboardSetter func(string)
}

func buildUI(state *uiState, ctx context.Context, client *api.Client, win fyne.Window) fyne.CanvasObject {
    state.searchEntry = widget.NewEntry()
    state.searchEntry.SetPlaceHolder("Search…")
    state.searchEntry.OnChanged = func(q string) {
        q = strings.ToLower(strings.TrimSpace(q))
        if q == "" {
            state.filtered = state.resources
        } else {
            state.filtered = nil
            for _, r := range state.resources {
                if strings.Contains(strings.ToLower(r.Name), q) {
                    state.filtered = append(state.filtered, r)
                }
            }
        }
        state.list.Refresh()
    }

    state.nameLabel = widget.NewLabel("")
    state.userLabel = widget.NewLabel("")
    state.uriLabel = widget.NewLabel("")

    state.passEntry = widget.NewPasswordEntry()
    state.passEntry.Disable()
    passBox := container.New(layout.NewMaxLayout(), state.passEntry)

    showBtn := widget.NewButtonWithIcon("Show", theme.VisibilityIcon(), func() {
        state.passEntry.Password = false
        state.passEntry.Refresh()
    })
    hideBtn := widget.NewButtonWithIcon("Hide", theme.VisibilityOffIcon(), func() {
        state.passEntry.Password = true
        state.passEntry.Refresh()
    })
    copyBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
        if state.clipboardSetter != nil {
            state.clipboardSetter(state.passEntry.Text)
        }
    })

    details := container.NewVBox(
        widget.NewLabelWithStyle("Details", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
        state.nameLabel,
        state.userLabel,
        state.uriLabel,
        container.NewHBox(widget.NewLabel("Password:"), passBox, showBtn, hideBtn, copyBtn),
    )

    state.filtered = state.resources
    state.list = widget.NewList(
        func() int { return len(state.filtered) },
        func() fyne.CanvasObject { return widget.NewLabel("") },
        func(i widget.ListItemID, o fyne.CanvasObject) {
            o.(*widget.Label).SetText(state.filtered[i].Name)
        },
    )

    state.list.OnSelected = func(id widget.ListItemID) {
        res := state.filtered[id]
        _, name, user, uri, pwd, _, err := helper.GetResource(ctx, client, res.ID)
        if err != nil {
            dialog.ShowError(err, win)
            return
        }
        state.nameLabel.SetText(fmt.Sprintf("Name: %s", name))
        state.userLabel.SetText(fmt.Sprintf("Username: %s", user))
        state.uriLabel.SetText(fmt.Sprintf("URI: %s", uri))
        state.passEntry.SetText(pwd)
        state.passEntry.Password = true
    }

    left := container.NewBorder(state.searchEntry, nil, nil, nil, state.list)
    split := container.NewHSplit(left, details)
    split.SetOffset(0.3)
    return split
}

func main() {
    ctx := context.Background()
    myApp := app.New()
    myApp.Settings().SetTheme(theme.DarkTheme())

    splash := myApp.NewWindow("Passbolt – Signing in…")
    splash.SetContent(widget.NewLabel("Connecting to Passbolt…"))
    splash.Resize(fyne.NewSize(300, 100))
    splash.Show()

    var (
        client    *api.Client
        resources []api.Resource
        loginErr  error
    )
    done := make(chan struct{})

    go func() {
        client, loginErr = newClient(ctx, myApp)
        if loginErr == nil {
            resources, loginErr = client.GetResources(ctx, nil)
        }
        close(done)
    }()

    go func() {
        <-done
        fyne.Do(func() {
            if loginErr != nil {
                dialog.ShowError(loginErr, splash)
                return
            }

            splash.Close()

            mainWin := myApp.NewWindow("Passbolt Viewer")
            mainWin.Resize(fyne.NewSize(900, 600))
            state := &uiState{
                resources: resources,
                clipboardSetter: func(t string) {
                    mainWin.Clipboard().SetContent(t)
                },
            }
            mainWin.SetContent(buildUI(state, ctx, client, mainWin))
            mainWin.Show()
        })
    }()

    myApp.Run()
}

