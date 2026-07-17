import { HttpInterceptorFn } from '@angular/common/http';
import { environment } from '../../environments/environment';

export const apiTokenInterceptor: HttpInterceptorFn = (req, next) => {
  if (!req.url.startsWith(environment.apiBaseUrl)) {
    return next(req);
  }
  const cloned = req.clone({
    setHeaders: { 'X-Api-Token': environment.apiToken },
  });
  return next(cloned);
};
