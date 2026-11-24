import { NavLink, Navigate, Route, Routes } from 'react-router-dom'
import { LabProvider } from './lab-context'
import { ModelPage } from '../features/model/ModelPage'
import { SimulationPage } from '../features/simulation/SimulationPage'
import { ComparisonPage } from '../features/comparison/ComparisonPage'
import { ExperimentsPage } from '../features/experiments/ExperimentsPage'
import { PresentationPage } from '../features/presentation/PresentationPage'

const navigation = [
  { to: '/presentation', label: 'Презентация защиты' },
  { to: '/model', label: 'Модель' },
  { to: '/search', label: 'Поиск равновесия' },
  { to: '/comparison', label: 'Сравнение' },
  { to: '/research', label: 'Исследование' },
]

function Application() {
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Перейти к содержанию</a>
      <header className="site-header">
        <div>
          <p className="eyebrow">Nash Load Lab</p>
          <p className="site-header__title">Распределение вычислительной нагрузки</p>
        </div>
        <nav aria-label="Основные разделы">
          <ul className="navigation">
            {navigation.map((item) => (
              <li key={item.to}>
                <NavLink to={item.to} className={({ isActive }) => isActive ? 'navigation__link navigation__link--active' : 'navigation__link'}>{item.label}</NavLink>
              </li>
            ))}
          </ul>
        </nav>
      </header>
      <main id="main-content" className="main-content">
        <Routes>
          <Route path="/presentation" element={<PresentationPage />} />
          <Route path="/model" element={<ModelPage />} />
          <Route path="/search" element={<SimulationPage />} />
          <Route path="/comparison" element={<ComparisonPage />} />
          <Route path="/research" element={<ExperimentsPage />} />
          <Route path="*" element={<Navigate to="/model" replace />} />
        </Routes>
      </main>
    </div>
  )
}

export function App() {
  return <LabProvider><Application /></LabProvider>
}
