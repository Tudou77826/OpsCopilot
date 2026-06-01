import React from 'react'
import {createRoot} from 'react-dom/client'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/700.css'
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
