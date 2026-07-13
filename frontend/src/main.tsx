import React from 'react'
import {createRoot} from 'react-dom/client'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/700.css'
import '@fontsource/fira-code/400.css'
import '@fontsource/fira-code/700.css'
import '@fontsource/source-code-pro/400.css'
import '@fontsource/source-code-pro/700.css'
import '@fontsource/ibm-plex-mono/400.css'
import '@fontsource/ibm-plex-mono/700.css'
import '@fontsource/inconsolata/400.css'
import '@fontsource/inconsolata/700.css'
import './style.css'
import 'flexlayout-react/style/dark.css'
import './components/FlexLayout/flexlayout-dark.css'
import App from './App'
import { ToastProvider } from './components/Toast/Toast'
import ConfirmDialogInternal from './components/ConfirmDialog/ConfirmDialog'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <ToastProvider>
            <App/>
            <ConfirmDialogInternal />
        </ToastProvider>
    </React.StrictMode>
)
