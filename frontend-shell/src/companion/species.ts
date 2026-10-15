import catalog from '../../../pkg/companion/species.json';
export const companionSpecies = catalog;
export function speciesInfo(id = 'lumen') { return companionSpecies.find(species => species.id === id) ?? companionSpecies[0]; }
