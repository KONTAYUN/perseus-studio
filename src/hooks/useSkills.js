import { useEffect, useState } from 'react';
import { apiDelete, apiGet, apiPatch, apiPost, apiPostForm } from '../api/client';

export default function useSkills({ enabled = true, setSettingsError = () => {} } = {}) {
  const [skills, setSkills] = useState([]);

  useEffect(() => {
    if (!enabled) return;
    loadSkills();
  }, [enabled]);

  async function loadSkills() {
    setSettingsError('');
    try {
      const data = await apiGet('/api/skills');
      setSkills(data.items || []);
    } catch (error) {
      console.error('loadSkills failed', error);
      setSettingsError(error.message);
    }
  }

  async function uploadSkillZip(file) {
    setSettingsError('');
    try {
      const formData = new FormData();
      formData.append('file', file, file.name);
      const result = await apiPostForm('/api/skills/upload', formData);
      await loadSkills();
      return result;
    } catch (error) {
      console.error('uploadSkillZip failed', error);
      setSettingsError(error.message);
      throw error;
    }
  }

  async function installSelectedSkills(uploadId, candidateIds) {
    setSettingsError('');
    try {
      const result = await apiPost('/api/skills/install-selected', {
        upload_id: uploadId,
        candidate_ids: candidateIds,
        enabled: true,
      });
      await loadSkills();
      return result;
    } catch (error) {
      console.error('installSelectedSkills failed', error);
      setSettingsError(error.message);
      throw error;
    }
  }

  async function updateSkill(id, input) {
    setSettingsError('');
    try {
      const item = await apiPatch(`/api/skills/${encodeURIComponent(id)}`, input);
      await loadSkills();
      return item;
    } catch (error) {
      console.error('updateSkill failed', error);
      setSettingsError(error.message);
      throw error;
    }
  }

  async function deleteSkill(id) {
    setSettingsError('');
    try {
      await apiDelete(`/api/skills/${encodeURIComponent(id)}`);
      await loadSkills();
    } catch (error) {
      console.error('deleteSkill failed', error);
      setSettingsError(error.message);
      throw error;
    }
  }

  async function readSkill(id) {
    return apiGet(`/api/skills/${encodeURIComponent(id)}/read`);
  }

  async function getSkill(id) {
    return apiGet(`/api/skills/${encodeURIComponent(id)}`);
  }

  return {
    deleteSkill,
    getSkill,
    installSelectedSkills,
    loadSkills,
    readSkill,
    skills,
    updateSkill,
    uploadSkillZip,
  };
}
