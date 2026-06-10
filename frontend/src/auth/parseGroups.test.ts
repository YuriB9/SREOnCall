import { describe, expect, it } from 'vitest'

import { parseGroups } from './usePermissions'

// Сценарии из спеки frontend-shell «Парсинг JWT claims» — зеркало серверных
// IsMember/IsAdmin (pkg/auth/auth.go).
describe('parseGroups', () => {
  it('членство в нескольких тенантах, admin перекрывает member', () => {
    expect(parseGroups(['/team-alpha', '/team-beta', '/team-beta/admins'])).toEqual({
      'team-alpha': 'member',
      'team-beta': 'admin',
    })
  })

  it('префиксная подгруппа без отдельной записи тенанта даёт member', () => {
    expect(parseGroups(['/team-gamma/oncall'])).toEqual({ 'team-gamma': 'member' })
  })

  it('admin не понижается до member последующей подгруппой', () => {
    expect(parseGroups(['/team-a/admins', '/team-a/oncall', '/team-a'])).toEqual({
      'team-a': 'admin',
    })
  })

  it('/{tenant}/admins/<подгруппа> — это member, не admin (зеркало IsAdmin)', () => {
    expect(parseGroups(['/team-a/admins/juniors'])).toEqual({ 'team-a': 'member' })
  })

  it('пустой массив групп', () => {
    expect(parseGroups([])).toEqual({})
  })

  it('отсутствующий аргумент (groups нет в токене)', () => {
    expect(parseGroups()).toEqual({})
  })

  it('мусорные записи не дают тенантов', () => {
    expect(parseGroups(['', '/', '//admins'])).toEqual({})
  })

  it('группа без ведущего слэша трактуется по первому сегменту', () => {
    expect(parseGroups(['team-solo'])).toEqual({ 'team-solo': 'member' })
  })
})
