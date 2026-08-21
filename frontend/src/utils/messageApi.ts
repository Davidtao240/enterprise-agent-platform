let messageApi: any = null;

export const setMessageApi = (api: any) => {
  messageApi = api;
};

export const getMessageApi = () => {
  return messageApi;
};

export const showError = (content: string, duration = 3) => {
  if (messageApi) {
    messageApi.error(content, duration);
  } else {
    import('antd').then(({ message }) => {
      message.error(content, duration);
    });
  }
};

export const showSuccess = (content: string, duration = 3) => {
  if (messageApi) {
    messageApi.success(content, duration);
  } else {
    import('antd').then(({ message }) => {
      message.success(content, duration);
    });
  }
};

export const showInfo = (content: string, duration = 3) => {
  if (messageApi) {
    messageApi.info(content, duration);
  } else {
    import('antd').then(({ message }) => {
      message.info(content, duration);
    });
  }
};
